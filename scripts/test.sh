#!/bin/bash
# test.sh - Comprehensive test suite for Docker checkpoint lab

set -e

# Color output functions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_test() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Test result tracking
declare -a FAILED_TESTS=()

# Function to run a test and track results
run_test() {
    local test_name="$1"
    local test_command="$2"

    TESTS_RUN=$((TESTS_RUN + 1))
    log_test "Running: $test_name"

    if eval "$test_command" >/dev/null 2>&1; then
        log_info "✓ PASSED: $test_name"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        log_error "✗ FAILED: $test_name"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        FAILED_TESTS+=("$test_name")
        return 1
    fi
}

# Function to cleanup test containers
cleanup_containers() {
    local containers=("test-basic" "test-network" "test-long-running" "test-multi-process" "test-memory")

    for container in "${containers[@]}"; do
        docker stop "$container" 2>/dev/null || true
        docker rm "$container" 2>/dev/null || true
    done

    # Clean up checkpoint directories
    sudo rm -rf /tmp/docker-checkpoints/test-* 2>/dev/null || true
}

# Test: Prerequisites check
test_prerequisites() {
    log_test "Checking prerequisites..."

    # Check if docker-checkpoint binary exists
    if [ ! -f "./docker-checkpoint" ]; then
        log_error "docker-checkpoint binary not found. Run 'go build' first."
        return 1
    fi

    # Check Docker access
    if ! docker ps >/dev/null 2>&1; then
        log_error "Cannot access Docker daemon"
        return 1
    fi

    # Check CRIU availability
    if ! command -v criu >/dev/null 2>&1; then
        log_error "CRIU not found"
        return 1
    fi

    # Check sudo access
    if ! sudo -n true 2>/dev/null; then
        log_error "Sudo access required for CRIU operations"
        return 1
    fi

    log_info "All prerequisites met"
    return 0
}

# Test: Basic container checkpoint
test_basic_checkpoint() {
    log_test "Testing basic container checkpoint..."

    # Start simple container
    docker run -d --name test-basic alpine sh -c 'counter=0; while true; do echo "Count: $counter"; counter=$((counter + 1)); sleep 1; done'
    sleep 2

    # Checkpoint the container
    if sudo timeout 60 ./docker-checkpoint -container test-basic -name basic-test; then
        # Check if checkpoint files were created
        if [ -d "/tmp/docker-checkpoints/test-basic/basic-test" ] && [ -f "/tmp/docker-checkpoints/test-basic/basic-test/inventory.img" ]; then
            return 0
        else
            log_error "Checkpoint files not found"
            return 1
        fi
    else
        log_error "Checkpoint command failed"
        return 1
    fi
}

# Test: Container with network service
test_network_checkpoint() {
    log_test "Testing container with network service..."

    # Start nginx container
    docker run -d --name test-network -p 8080:80 nginx
    sleep 5

    # Checkpoint with TCP handling
    if sudo timeout 60 ./docker-checkpoint -container test-network -name network-test -tcp=true; then
        if [ -f "/tmp/docker-checkpoints/test-network/network-test/netdev-*.img" 2>/dev/null ] || [ -f "/tmp/docker-checkpoints/test-network/network-test/inventory.img" ]; then
            return 0
        else
            return 1
        fi
    else
        return 1
    fi
}

# Test: Long running container
test_long_running() {
    log_test "Testing long-running container checkpoint..."

    # Start container that runs for a while
    docker run -d --name test-long-running alpine sh -c 'for i in $(seq 1 1000); do echo "Iteration $i"; sleep 0.1; done'
    sleep 2

    # Checkpoint and stop
    if sudo timeout 60 ./docker-checkpoint -container test-long-running -name long-running-test -leave-running=false; then
        # Check if container stopped
        if ! docker ps | grep -q test-long-running; then
            return 0
        else
            log_warn "Container should have stopped but is still running"
            return 1
        fi
    else
        return 1
    fi
}

# Test: Multi-process container
test_multi_process() {
    log_test "Testing multi-process container..."

    # Start container with multiple processes
    docker run -d --name test-multi-process ubuntu sh -c 'sleep 3600 & sleep 3600 & wait'
    sleep 3

    # Checkpoint multi-process container
    if sudo timeout 60 ./docker-checkpoint -container test-multi-process -name multi-process-test; then
        if [ -f "/tmp/docker-checkpoints/test-multi-process/multi-process-test/pstree.img" ]; then
            return 0
        else
            return 1
        fi
    else
        return 1
    fi
}

# Test: Memory-intensive container
test_memory_usage() {
    log_test "Testing memory-intensive container..."

    # Start container that uses some memory
    docker run -d --name test-memory alpine sh -c '
        head -c 50M /dev/zero > /tmp/memfile
        while true; do
            cat /tmp/memfile > /dev/null
            sleep 1
        done
    '
    sleep 5

    # Checkpoint memory-intensive container
    if sudo timeout 60 ./docker-checkpoint -container test-memory -name memory-test; then
        # Check if pages file exists and has reasonable size
        if [ -f "/tmp/docker-checkpoints/test-memory/memory-test/pages-1.img" ]; then
            local size=$(stat -c%s "/tmp/docker-checkpoints/test-memory/memory-test/pages-1.img" 2>/dev/null || echo "0")
            if [ "$size" -gt 1000000 ]; then  # At least 1MB
                return 0
            else
                log_warn "Memory checkpoint seems too small: $size bytes"
                return 1
            fi
        else
            return 1
        fi
    else
        return 1
    fi
}

# Test: Pre-dump functionality
test_predump() {
    log_test "Testing pre-dump functionality..."

    # Start container
    docker run -d --name test-predump alpine sleep 3600
    sleep 2

    # Test pre-dump
    if sudo timeout 60 ./docker-checkpoint -container test-predump -name predump-test -pre-dump=true; then
        return 0
    else
        return 1
    fi
}

# Test: Custom checkpoint directory
test_custom_directory() {
    log_test "Testing custom checkpoint directory..."

    # Start container
    docker run -d --name test-custom alpine sleep 30
    sleep 2

    # Create custom directory
    local custom_dir="/tmp/custom-checkpoints"
    mkdir -p "$custom_dir"

    # Checkpoint to custom directory
    if sudo timeout 60 ./docker-checkpoint -container test-custom -name custom-test -dir "$custom_dir"; then
        if [ -d "$custom_dir/test-custom/custom-test" ]; then
            sudo rm -rf "$custom_dir"
            return 0
        else
            return 1
        fi
    else
        sudo rm -rf "$custom_dir"
        return 1
    fi
}

# Test: Error handling
test_error_handling() {
    log_test "Testing error handling..."

    # Test with non-existent container
    if sudo ./docker-checkpoint -container non-existent-container -name error-test 2>/dev/null; then
        log_error "Should have failed with non-existent container"
        return 1
    fi

    # Test with stopped container
    docker run --name test-stopped alpine echo "hello"
    if sudo ./docker-checkpoint -container test-stopped -name error-test 2>/dev/null; then
        log_error "Should have failed with stopped container"
        docker rm test-stopped
        return 1
    fi

    docker rm test-stopped
    return 0
}

# Test: Application help and version
test_application_interface() {
    log_test "Testing application interface..."

    # Test help output
    if ./docker-checkpoint -h 2>&1 | grep -q "Usage"; then
        return 0
    else
        return 1
    fi
}

# Print test summary
print_summary() {
    echo
    echo "=== Test Summary ==="
    echo "Tests Run:    $TESTS_RUN"
    echo "Tests Passed: $TESTS_PASSED"
    echo "Tests Failed: $TESTS_FAILED"

    if [ $TESTS_FAILED -gt 0 ]; then
        echo
        log_error "Failed Tests:"
        for test in "${FAILED_TESTS[@]}"; do
            echo "  - $test"
        done
        echo
        log_warn "Check the troubleshooting section in README.md for help"
    fi

    echo
    if [ $TESTS_FAILED -eq 0 ]; then
        log_info "🎉 All tests passed! Your checkpoint system is working correctly."
    else
        log_warn "Some tests failed. The basic functionality may still work."
    fi
}

# Main test execution
main() {
    echo "=== Docker Container Checkpoint Test Suite ==="
    echo

    # Cleanup any existing test containers
    log_info "Cleaning up any existing test containers..."
    cleanup_containers

    # Run tests
    log_info "Starting comprehensive test suite..."
    echo

    # Prerequisites must pass
    if ! test_prerequisites; then
        log_error "Prerequisites check failed. Cannot continue testing."
        exit 1
    fi

    # Core functionality tests
    run_test "Application Interface" "test_application_interface"
    run_test "Basic Container Checkpoint" "test_basic_checkpoint"
    run_test "Network Service Container" "test_network_checkpoint"
    run_test "Long Running Container" "test_long_running"
    run_test "Multi-Process Container" "test_multi_process"
    run_test "Memory-Intensive Container" "test_memory_usage"
    run_test "Pre-dump Functionality" "test_predump"
    run_test "Custom Directory" "test_custom_directory"
    run_test "Error Handling" "test_error_handling"

    # Cleanup
    log_info "Cleaning up test containers..."
    cleanup_containers

    # Print results
    print_summary

    # Exit with appropriate code
    if [ $TESTS_FAILED -eq 0 ]; then
        exit 0
    else
        exit 1
    fi
}

# Check if running as root
if [ "$EUID" -eq 0 ]; then
    log_error "Do not run this test script as root. Use a regular user with sudo access."
    exit 1
fi

# Run main function
main "$@"