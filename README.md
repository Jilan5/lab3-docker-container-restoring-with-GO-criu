# Docker Container Checkpoint with Go-CRIU

## Introduction

This project demonstrates how to checkpoint Docker containers using CRIU (Checkpoint/Restore In Userspace). The implementation is a  Go application that can inspect and checkpoint running Docker containers, understanding the complete flow from Docker API integration to CRIU system calls.



## Objectives
By using this project, you will understand:

1. **Docker Container Internals**: How to inspect and extract container metadata
2. **CRIU Integration**: How to configure and use CRIU programmatically
3. **Mount Point Management**: How Docker's filesystem layers work with checkpointing
4. **Error Handling**: How to debug and troubleshoot checkpoint failures
5. **Production Deployment**: How to deploy and manage checkpoint systems

## Prerequisites

### System Requirements
- Ubuntu 20.04 or 22.04 (recommended)
- Minimum 2 CPU cores and 4GB RAM
- Docker with experimental features enabled
- CRIU (Checkpoint/Restore In Userspace)
- Go 1.19+
- Root/sudo privileges

### Knowledge Prerequisites
- Basic understanding of Docker containers
- Familiarity with Go programming
- Linux system administration basics
- Understanding of processes and namespaces

## Quick Setup

### Automated Setup (Recommended)

```bash
# Clone the repository
git clone https://github.com/Jilan5/Docker-container-checkpointing-with-Go-Criu.git
cd Docker-container-checkpointing-with-Go-Criu

# Run the automated setup script
chmod +x scripts/setup.sh
./scripts/setup.sh
```

The setup script will:
1. Update system packages
2. Install Docker, CRIU, and Go if not present
3. Enable Docker experimental features
4. Build the checkpoint application
5. Add Go to your PATH
6. Run a simple test to verify everything works

### Manual Setup

If you prefer manual installation:

```bash
# Update system
sudo apt-get update
sudo apt-get install -y wget curl git build-essential

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh
sudo usermod -aG docker $USER

# Install CRIU
sudo apt-get install -y criu
sudo setcap cap_sys_admin,cap_sys_ptrace,cap_sys_chroot+ep $(which criu)

# Install Go
GO_VERSION="1.21.5"
wget "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Enable Docker experimental features
sudo mkdir -p /etc/docker
echo '{"experimental": true}' | sudo tee /etc/docker/daemon.json
sudo systemctl restart docker

# Build the application
go mod tidy
go build -o docker-checkpoint
```

## Usage

### Basic Checkpoint

```bash
# Start a test container
docker run -d --name test-app alpine sh -c 'counter=0; while true; do echo "Count: $counter"; counter=$((counter + 1)); sleep 1; done'

# Checkpoint the container (leaves it running)
sudo ./docker-checkpoint -container test-app -name checkpoint1

# Check the checkpoint files
ls -la /tmp/docker-checkpoints/test-app/checkpoint1/
```

### Command Line Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `-container` | `string` | *required* | Container name or ID to checkpoint |
| `-name` | `string` | `checkpoint1` | Name for the checkpoint |
| `-dir` | `string` | `/tmp/docker-checkpoints` | Base directory for checkpoints |
| `-leave-running` | `boolean` | `true` | Leave container running after checkpoint |
| `-tcp` | `boolean` | `true` | Checkpoint established TCP connections |
| `-file-locks` | `boolean` | `true` | Checkpoint file locks |
| `-pre-dump` | `boolean` | `false` | Perform pre-dump for optimization |

**Usage:**
```bash
sudo ./docker-checkpoint -container <name> [options]
```

### Advanced Examples

```bash
# Checkpoint and stop the container
sudo ./docker-checkpoint -container myapp -leave-running=false

# Checkpoint with custom directory
sudo ./docker-checkpoint -container myapp -dir /opt/checkpoints

# Checkpoint with pre-dump optimization
sudo ./docker-checkpoint -container myapp -pre-dump=true

# Checkpoint a web server with TCP connections
docker run -d --name nginx -p 8080:80 nginx
sudo ./docker-checkpoint -container nginx -tcp=true
```

## Architecture & Implementation

### Core Components

1. **Container Inspection**
   - Connects to Docker API using Docker client
   - Extracts container metadata (PID, namespaces, mounts, cgroups)
   - Validates container is running

2. **CRIU Configuration**
   - Sets up CRIU options for Docker containers
   - Configures external mount handling for Docker's bind mounts
   - Manages cgroup and namespace settings

3. **Checkpoint Execution**
   - Performs the actual CRIU dump operation
   - Handles pre-dump optimization if requested
   - Saves container metadata for future restoration

4. **Error Handling**
   - Provides detailed error messages
   - Shows CRIU logs on failure
   - Validates prerequisites

### Go Code Implementation

#### Main Function Flow
The application follows a structured approach:

```go
func main() {
    // Parse command line flags using Go's flag package
    flag.StringVar(&containerName, "container", "", "Container name or ID to checkpoint")
    flag.BoolVar(&leaveRunning, "leave-running", true, "Leave container running after checkpoint")
    // ... other flags

    // Create options struct and call checkpoint function
    opts := Options{LeaveRunning: leaveRunning, TCPEstablished: tcpEstablished, ...}
    checkpointContainer(containerName, checkpointName, baseDir, opts)
}
```

#### Docker API Integration
The `inspectContainer()` function demonstrates Docker API usage:

```go
func inspectContainer(containerName string) (*ContainerInfo, error) {
    // Create Docker client using environment variables (DOCKER_HOST, etc.)
    cli, err := client.NewClientWithOpts(client.FromEnv)

    // Inspect container to get detailed information
    containerJSON, err := cli.ContainerInspect(ctx, containerName)

    // Extract critical information for checkpointing
    info := &ContainerInfo{
        ID:         containerJSON.ID[:12],                              // Short container ID
        PID:        containerJSON.State.Pid,                          // Main process PID
        RootFS:     containerJSON.GraphDriver.Data["MergedDir"],      // Container filesystem root
        Runtime:    containerJSON.HostConfig.Runtime,                 // Container runtime (runc/containerd)
    }

    // Map all Linux namespaces for the container process
    for _, ns := range []string{"ipc", "mnt", "net", "pid", "user", "uts", "cgroup"} {
        info.Namespaces[ns] = fmt.Sprintf("/proc/%d/ns/%s", info.PID, ns)
    }
}
```

#### CRIU Configuration and Execution
The `doCRIUCheckpoint()` function shows CRIU library usage:

```go
func doCRIUCheckpoint(info *ContainerInfo, checkpointDir string, opts Options) error {
    // Initialize CRIU client from go-criu library
    criuClient := criu.MakeCriu()
    criuClient.SetCriuPath("criu")

    // Configure CRIU options using protocol buffers
    criuOpts := &rpc.CriuOpts{
        Pid:            proto.Int32(int32(info.PID)),        // Target process PID
        LogLevel:       proto.Int32(4),                      // Verbose logging
        Root:           proto.String(info.RootFS),           // Container root filesystem
        ManageCgroups:  proto.Bool(true),                    // Handle cgroup hierarchy
        ShellJob:       proto.Bool(true),                    // Required for docker containers

        // Mark Docker-managed mounts as external (don't checkpoint them)
        External: []string{
            "mnt[/proc]:proc",         // Process filesystem
            "mnt[/dev]:dev",           // Device filesystem
            "mnt[/etc/hostname]:hostname", // Docker networking files
            // ... other Docker bind mounts
        },
    }

    // Execute the checkpoint operation
    return criuClient.Dump(criuOpts, nil)
}
```

#### Key Go Patterns Used

1. **Struct-based Configuration**: Uses `ContainerInfo` and `Options` structs for clean data organization
2. **Error Wrapping**: Implements Go 1.13+ error wrapping with `fmt.Errorf("context: %w", err)`
3. **Context Management**: Uses `context.Background()` for Docker API calls
4. **Protocol Buffers**: Leverages protobuf for CRIU RPC communication
5. **File Operations**: Standard `os` and `filepath` packages for directory management

### Container Information Structure

```go
type ContainerInfo struct {
    ID         string                 // Short container ID
    Name       string                 // Container name
    PID        int                   // Main process PID
    State      string                // Container state
    RootFS     string                // Root filesystem path
    Runtime    string                // Container runtime (runc, etc.)
    BundlePath string                // OCI bundle path
    Namespaces map[string]string     // Process namespaces
    CgroupPath string                // Cgroup path
}
```

### CRIU Configuration

The application configures CRIU with Docker-specific options:

```go
criuOpts := &rpc.CriuOpts{
    Pid:            proto.Int32(int32(info.PID)),
    LogLevel:       proto.Int32(4),           // Verbose logging
    LogFile:        proto.String("dump.log"),
    Root:           proto.String(info.RootFS),
    ManageCgroups:  proto.Bool(true),         // Handle cgroups
    TcpEstablished: proto.Bool(true),         // Checkpoint TCP connections
    FileLocks:      proto.Bool(true),         // Handle file locks
    LeaveRunning:   proto.Bool(true),         // Keep container running
    ShellJob:       proto.Bool(true),         // Required for docker run containers
}
```

### Mount Point Handling

Docker containers have complex mount structures that require special handling:

```go
External: []string{
    "mnt[/proc]:proc",              // Process filesystem
    "mnt[/dev]:dev",                // Device filesystem
    "mnt[/sys]:sys",                // System filesystem
    "mnt[/dev/shm]:shm",            // Shared memory
    "mnt[/dev/pts]:pts",            // Pseudo terminals
    "mnt[/dev/mqueue]:mqueue",      // Message queues
    "mnt[/etc/hostname]:hostname",   // Docker bind mounts
    "mnt[/etc/hosts]:hosts",        // Docker bind mounts
    "mnt[/etc/resolv.conf]:resolv.conf", // DNS configuration
    "mnt[/sys/fs/cgroup]:cgroup",   // Cgroup filesystem
}
```

## Understanding Checkpoint Files

After a successful checkpoint, you'll find these files:

| File | Purpose |
|------|---------|
| `core-*.img` | Process core information and registers |
| `pages-*.img` | Memory page contents |
| `pagemap-*.img` | Memory page mappings |
| `fdinfo-*.img` | File descriptor information |
| `mountpoints-*.img` | Mount point information |
| `netdev-*.img` | Network device state |
| `container.json` | Container metadata (custom) |
| `dump.log` | CRIU operation log |

## Testing

### Automated Testing

Run the comprehensive test suite:
```bash
chmod +x scripts/test.sh
sudo ./scripts/test.sh
```

This will:
1. Start various test containers (Alpine, Nginx, Python apps)
2. Attempt to checkpoint them with different configurations
3. Verify checkpoint files are created and valid
4. Test error handling and edge cases
5. Clean up test containers and checkpoints

### Manual Testing

Test with different container types:

```bash
# Simple Alpine container
docker run -d --name test-simple alpine sleep 3600
sudo ./docker-checkpoint -container test-simple

# Container with network service
docker run -d --name test-nginx -p 8080:80 nginx
sudo ./docker-checkpoint -container test-nginx

# Container with mounted volumes
docker run -d --name test-volume -v /tmp:/data alpine sh -c 'while true; do date > /data/timestamp; sleep 1; done'
sudo ./docker-checkpoint -container test-volume
```

## Troubleshooting

### Common Issues

#### 1. Permission Denied
```bash
# Ensure CRIU has proper capabilities
sudo setcap cap_sys_admin,cap_sys_ptrace,cap_sys_chroot+ep $(which criu)

# Run with sudo
sudo ./docker-checkpoint -container myapp
```

#### 2. Docker Experimental Features
```bash
# Verify experimental features are enabled
docker version | grep Experimental

# If not enabled, run:
sudo mkdir -p /etc/docker
echo '{"experimental": true}' | sudo tee /etc/docker/daemon.json
sudo systemctl restart docker
```

#### 3. Mount Point Errors
The application handles most Docker mount issues automatically. If you encounter mount errors, check the CRIU log:
```bash
cat /tmp/docker-checkpoints/<container>/<checkpoint>/dump.log
```

#### 4. Container Not Found
```bash
# List running containers
docker ps

# Use exact container name or ID
sudo ./docker-checkpoint -container exact_name
```

### Debug Mode

For detailed debugging, examine the CRIU log file:
```bash
# View recent checkpoint log
tail -f /tmp/docker-checkpoints/<container>/<checkpoint>/dump.log
```

## Common Challenges and Solutions

### Challenge 1: Mount Point Complexity

**Problem**: Docker's complex mount structure causes CRIU failures

**Solution**:
- Analyze mount structure with `docker inspect`
- Use external mount configuration for Docker-managed mounts
- Test mount handling with different container configurations

### Challenge 2: Cgroup Management

**Problem**: CRIU fails to handle Docker's cgroup hierarchy

**Solution**:
- Configure `ManageCgroups: true`
- Set proper cgroup root paths
- Handle both v1 and v2 cgroup configurations

### Challenge 3: Process Tree Complexity

**Problem**: Multi-process containers may have complex process trees

**Solution**:
- Use `ShellJob: true` for containers started with `docker run`
- Handle process hierarchies correctly
- Test with applications that spawn child processes

### Challenge 4: Network State

**Problem**: Network connections may prevent successful checkpoint

**Solution**:
- Configure `TcpEstablished: true` for network services
- Handle external network dependencies
- Test with various network configurations

## Security Considerations

- **Root Privileges**: This tool requires root/sudo for CRIU operations
- **Memory Contents**: Checkpoint files contain full memory dumps
- **Storage**: Store checkpoints securely with appropriate access controls
- **Cleanup**: Regularly clean old checkpoint files

## Limitations

- **Container Types**: Works with standard Docker containers
- **External Dependencies**: Some applications may have external state not captured
- **Network**: Complex network configurations may need additional handling
- **Volumes**: Persistent volumes are not checkpointed

## Next Steps

After completing this lab:

1. **Restoration**: Implement checkpoint restoration functionality
2. **Compression**: Add checkpoint compression to reduce file sizes
3. **Remote Storage**: Integrate with cloud storage for checkpoint persistence
4. **Automation**: Build CI/CD pipelines with checkpoint/restore
5. **Kubernetes**: Extend to Kubernetes pod checkpointing

## Resources

- [CRIU Documentation](https://criu.org/Documentation)
- [Docker Checkpoint Documentation](https://docs.docker.com/engine/reference/commandline/checkpoint/)
- [Go CRIU Library](https://github.com/checkpoint-restore/go-criu)
- [Container Runtime Specification](https://github.com/opencontainers/runtime-spec)

