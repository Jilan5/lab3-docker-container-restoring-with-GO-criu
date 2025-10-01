# Lab 2: Docker Container Checkpoint Implementation Guide

## Objective

Build a production-ready Go application that can checkpoint Docker containers using CRIU, understanding the complete flow from Docker API integration to CRIU system calls.

## Learning Outcomes

By the end of this lab, you will understand:

1. **Docker Container Internals**: How to inspect and extract container metadata
2. **CRIU Integration**: How to configure and use CRIU programmatically
3. **Mount Point Management**: How Docker's filesystem layers work with checkpointing
4. **Error Handling**: How to debug and troubleshoot checkpoint failures
5. **Production Deployment**: How to deploy and manage checkpoint systems

## Prerequisites

### System Requirements
- Ubuntu 20.04 or 22.04 (recommended for EC2)
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

## Implementation Deep Dive

### Part 1: Docker Container Inspection

The application needs to gather comprehensive container information before checkpointing:

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

**Key Implementation Points:**

1. **Docker API Connection**: Use the official Docker Go client with environment-based configuration
2. **Container Validation**: Ensure the container is running before attempting checkpoint
3. **Namespace Discovery**: Map all container namespaces for CRIU
4. **Path Resolution**: Extract correct filesystem and bundle paths

### Part 2: CRIU Configuration

CRIU requires specific configuration for Docker containers:

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

**Critical Configuration:**

1. **External Mounts**: Docker uses bind mounts that must be marked as external
2. **Cgroup Management**: CRIU must understand Docker's cgroup hierarchy
3. **Namespace Handling**: All container namespaces must be properly configured
4. **Filesystem Root**: Set the correct container root filesystem

### Part 3: Mount Point Handling

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

**Mount Strategy:**
- **External Mounts**: Mark system and Docker-managed mounts as external
- **Bind Mounts**: Handle Docker's special bind mounts for networking
- **Overlays**: Work with Docker's overlay filesystem layers

### Part 4: Error Handling and Debugging

Comprehensive error handling is crucial for production use:

1. **CRIU Log Analysis**: Parse and display CRIU logs on failure
2. **Container State Validation**: Verify container prerequisites
3. **Filesystem Checks**: Ensure all required paths exist
4. **Permission Validation**: Check for required capabilities

## Implementation Walkthrough

### Step 1: Container Discovery and Validation

```go
func inspectContainer(containerName string) (*ContainerInfo, error) {
    ctx := context.Background()
    cli, err := client.NewClientWithOpts(client.FromEnv)
    if err != nil {
        return nil, fmt.Errorf("failed to create docker client: %w", err)
    }

    containerJSON, err := cli.ContainerInspect(ctx, containerName)
    if err != nil {
        return nil, fmt.Errorf("failed to inspect container: %w", err)
    }

    if !containerJSON.State.Running {
        return nil, fmt.Errorf("container %s is not running", containerName)
    }

    // Extract and validate container information...
}
```

### Step 2: CRIU Configuration and Execution

```go
func doCRIUCheckpoint(info *ContainerInfo, checkpointDir string, opts Options) error {
    criuClient := criu.MakeCriu()
    criuClient.SetCriuPath("criu")

    // Configure CRIU options...
    criuOpts := &rpc.CriuOpts{
        // Configuration as shown above...
    }

    // Set working directory
    workDir, err := os.Open(checkpointDir)
    if err != nil {
        return fmt.Errorf("failed to open checkpoint directory: %w", err)
    }
    defer workDir.Close()

    criuOpts.ImagesDirFd = proto.Int32(int32(workDir.Fd()))

    // Execute checkpoint
    if err := criuClient.Dump(criuOpts, nil); err != nil {
        // Handle errors with detailed logging...
    }
}
```

### Step 3: Metadata Persistence

```go
func saveMetadata(info *ContainerInfo, checkpointDir string) error {
    metadata := map[string]interface{}{
        "id":          info.ID,
        "name":        info.Name,
        "runtime":     info.Runtime,
        "rootfs":      info.RootFS,
        "bundle_path": info.BundlePath,
        "namespaces":  info.Namespaces,
        "cgroup_path": info.CgroupPath,
        "timestamp":   time.Now().Format(time.RFC3339),
    }

    metadataFile := filepath.Join(checkpointDir, "container.json")
    file, err := os.Create(metadataFile)
    if err != nil {
        return err
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    encoder.SetIndent("", "  ")
    return encoder.Encode(metadata)
}
```

## Testing Strategy

### Unit Testing

1. **Container Inspection**: Test with various container configurations
2. **CRIU Options**: Validate configuration generation
3. **Error Handling**: Test failure scenarios

### Integration Testing

1. **Simple Containers**: Start with basic Alpine containers
2. **Network Containers**: Test containers with network services
3. **Complex Applications**: Test multi-process applications

### Production Testing

1. **Load Testing**: Checkpoint multiple containers simultaneously
2. **Resource Testing**: Test with memory and CPU intensive containers
3. **Failure Recovery**: Test error scenarios and recovery

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

## Best Practices

### Development

1. **Error Logging**: Always log CRIU output for debugging
2. **Validation**: Validate all inputs and prerequisites
3. **Testing**: Test with diverse container configurations
4. **Documentation**: Document configuration choices and limitations

### Production

1. **Monitoring**: Monitor checkpoint success rates and performance
2. **Storage**: Implement proper checkpoint file management
3. **Security**: Secure checkpoint files (they contain memory dumps)
4. **Automation**: Integrate with orchestration systems

### Performance

1. **Pre-dump**: Use pre-dump for large containers
2. **Compression**: Implement checkpoint compression
3. **Parallel Processing**: Handle multiple checkpoints concurrently
4. **Resource Management**: Monitor and limit resource usage

## Advanced Topics

### Checkpoint Optimization

1. **Memory Tracking**: Use CRIU's memory tracking features
2. **Incremental Checkpoints**: Implement incremental checkpoint strategies
3. **Compression**: Add checkpoint file compression
4. **Deduplication**: Implement memory page deduplication

### Integration Patterns

1. **Container Orchestration**: Integrate with Kubernetes
2. **CI/CD Pipelines**: Use checkpoints in testing pipelines
3. **Disaster Recovery**: Implement checkpoint-based backup strategies
4. **Migration**: Use checkpoints for container migration

### Troubleshooting Guide

#### Debug Workflow

1. **Check Prerequisites**: Verify CRIU installation and capabilities
2. **Examine Logs**: Review detailed CRIU logs
3. **Test Incrementally**: Start with simple containers
4. **Validate Configuration**: Verify all CRIU options
5. **Check Permissions**: Ensure proper privileges

#### Common Error Patterns

1. **Mount Errors**: Usually related to external mount configuration
2. **Permission Errors**: CRIU requires specific capabilities
3. **Process Errors**: Related to process tree complexity
4. **Network Errors**: TCP connection handling issues

## Assessment Criteria

Students will be evaluated on:

1. **Implementation Quality**: Clean, well-structured code
2. **Error Handling**: Comprehensive error handling and logging
3. **Testing**: Thorough testing with various scenarios
4. **Documentation**: Clear documentation and code comments
5. **Production Readiness**: Consideration of production concerns

## Next Lab Preview

Lab 3 will cover:
- **Checkpoint Restoration**: Implementing the restore functionality
- **State Management**: Managing checkpoint metadata and storage
- **Advanced Scenarios**: Handling stateful applications and databases
- **Performance Optimization**: Improving checkpoint and restore performance

## Resources for Further Learning

1. **CRIU Documentation**: https://criu.org/Documentation
2. **Docker Internals**: https://docs.docker.com/get-started/overview/
3. **Go CRIU Library**: https://github.com/checkpoint-restore/go-criu
4. **Container Runtime Specification**: https://github.com/opencontainers/runtime-spec