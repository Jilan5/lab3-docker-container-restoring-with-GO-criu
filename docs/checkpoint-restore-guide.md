# Docker Container Checkpoint/Restore Documentation

## Overview
This Go application provides functionality to checkpoint running Docker containers using CRIU (Checkpoint/Restore In Userspace) and restore them later, preserving their state and execution context.

## Prerequisites
- Docker installed and running
- CRIU installed (`apt-get install criu` or `yum install criu`)
- Go 1.19+ installed
- Root/sudo privileges (required for CRIU operations)
- Linux kernel with checkpoint/restore support enabled

## Building the Application

```bash
# Install dependencies
go mod download

# Build the application
go build -o docker-checkpoint main.go
```

## Usage

### Checkpoint a Running Container

To create a checkpoint of a running container:

```bash
sudo ./docker-checkpoint \
  -container <container_name> \
  -name <checkpoint_name> \
  -dir <checkpoint_directory> \
  -leave-running=true
```

**Parameters:**
- `-container`: Name or ID of the container to checkpoint
- `-name`: Name for the checkpoint (default: "checkpoint1")
- `-dir`: Base directory for storing checkpoints (default: "/tmp/docker-checkpoints")
- `-leave-running`: Keep container running after checkpoint (default: true)
- `-tcp`: Checkpoint TCP connections (default: true)
- `-file-locks`: Checkpoint file locks (default: true)
- `-pre-dump`: Perform pre-dump optimization (default: false)

**Example:**
```bash
# Checkpoint a container named 'myapp'
sudo ./docker-checkpoint -container myapp -name checkpoint1
```

### Restore a Container from Checkpoint

To restore a container from a previously created checkpoint:

```bash
sudo ./docker-checkpoint \
  -restore \
  -container <original_container_name> \
  -name <checkpoint_name> \
  -new-name <restored_container_name>
```

**Parameters:**
- `-restore`: Enable restore mode
- `-container`: Name of the original container (used to locate checkpoint)
- `-name`: Name of the checkpoint to restore from
- `-new-name`: New name for restored container (optional, defaults to "<original>-restored")

**Example:**
```bash
# Restore from checkpoint
sudo ./docker-checkpoint -restore -container myapp -name checkpoint1 -new-name myapp-restored
```

## Verification Process

The application automatically verifies restoration by:

1. **Container Status Check**: Inspects the restored container's state
2. **Process Verification**: Confirms the container process is running with a valid PID
3. **API Response Test**: Verifies the container responds to Docker API calls
4. **Log Retrieval**: Attempts to fetch recent container logs
5. **Stats Collection**: Confirms the container is generating runtime statistics

### Manual Verification Commands

After restoration, you can manually verify the container state:

```bash
# Check container status
docker ps -a | grep <container_name>

# Inspect container details
docker inspect <container_name>

# View container logs
docker logs --tail 20 <container_name>

# Execute command in restored container
docker exec <container_name> ps aux

# Check container stats
docker stats --no-stream <container_name>
```

## Checkpoint Storage Structure

Checkpoints are stored in the following structure:
```
<base_dir>/
└── <container_name>/
    └── <checkpoint_name>/
        ├── container.json     # Container metadata
        ├── core-*.img        # Process core dumps
        ├── fdinfo-*.img      # File descriptor information
        ├── pagemap-*.img     # Memory pages mapping
        ├── pages-*.img       # Memory pages data
        ├── dump.log          # CRIU dump log
        └── restore.log       # CRIU restore log (after restore)
```

## Example Workflow

```bash
# 1. Start a test container
docker run -d --name testapp alpine:latest sh -c "i=0; while true; do echo Counter: $i; i=$((i+1)); sleep 2; done"

# 2. Verify container is running
docker logs --tail 5 testapp

# 3. Create checkpoint (container continues running)
sudo ./docker-checkpoint -container testapp -name checkpoint1

# 4. Stop and remove original container (optional)
docker stop testapp
docker rm testapp

# 5. Restore container from checkpoint
sudo ./docker-checkpoint -restore -container testapp -name checkpoint1 -new-name testapp-restored

# 6. Verify restored container is running
docker ps | grep testapp-restored

# 7. Check if counter continued from checkpoint
docker logs --tail 10 testapp-restored
```

## Troubleshooting

### Common Issues

1. **Permission Denied**
   - Ensure running with sudo/root privileges
   - Check CRIU capabilities: `sudo criu check`

2. **CRIU Not Found**
   - Install CRIU: `sudo apt-get install criu`
   - Verify installation: `criu --version`

3. **Checkpoint Failed**
   - Check kernel support: `cat /proc/config.gz | gunzip | grep CONFIG_CHECKPOINT_RESTORE`
   - Review CRIU logs in checkpoint directory: `dump.log`

4. **Restore Failed**
   - Ensure checkpoint directory exists and contains valid checkpoint files
   - Check restore.log in checkpoint directory for detailed errors
   - Verify no container with the same name already exists

### Debug Information

To get detailed debug output:

```bash
# Check CRIU capabilities
sudo criu check

# View checkpoint logs
cat /tmp/docker-checkpoints/<container>/<checkpoint>/dump.log

# View restore logs
cat /tmp/docker-checkpoints/<container>/<checkpoint>/restore.log

# Check kernel configuration
zcat /proc/config.gz | grep -E "CHECKPOINT|RESTORE"
```

## Limitations

- Only works on Linux with checkpoint/restore kernel support
- Requires root/sudo privileges
- Some container features may not be fully supported (GPU, certain network configurations)
- Containers with open network connections may require special handling
- Shared memory and IPC resources need careful management

## Security Considerations

- Checkpoint files contain complete memory dumps of processes
- Store checkpoints securely as they may contain sensitive data
- Use appropriate file permissions on checkpoint directories
- Consider encrypting checkpoint data for sensitive applications