# Diagram Prompts for Docker Container Checkpoint with Go-CRIU Project

## Diagram 1: System Architecture Overview

**Prompt for Gemini:**
```
Create a system architecture diagram showing:
- User/CLI at the top
- Go Application (docker-checkpoint) in the middle containing:
  - Docker API Client module
  - CRIU Configuration module
  - Checkpoint Execution module
  - Error Handling module
- Docker Daemon on the left side with running containers
- CRIU binary on the right side
- File system at the bottom showing checkpoint storage (/tmp/docker-checkpoints/)
- Show data flow arrows between components
- Include labels for key technologies: Go, Docker API, CRIU, Linux namespaces
- Use a clean, professional style with boxes and arrows
```

## Diagram 2: Checkpoint Process Flow

**Prompt for Gemini:**
```
Create a flowchart diagram showing the checkpoint process:
1. Start: User runs docker-checkpoint command
2. Parse command line arguments (container name, options)
3. Connect to Docker API
4. Inspect container (get PID, namespaces, mounts, cgroups)
5. Validate container is running
6. Create checkpoint directory
7. Configure CRIU options (external mounts, cgroups, TCP settings)
8. Execute CRIU dump operation
9. Save container metadata to JSON
10. End: Display success message and checkpoint location
- Include decision diamonds for validation steps
- Show error paths in red
- Use different colors for different phases (inspection, configuration, execution)
- Make it vertical flow from top to bottom
```

## Diagram 3: Container Information Structure

**Prompt for Gemini:**
```
Create a detailed diagram showing container information extraction:
- Docker Container (visual container icon) at the top
- Docker API call arrow pointing down
- ContainerJSON structure in the middle showing:
  - ID (containerJSON.ID)
  - Name (containerJSON.Name)
  - State.Pid (main process)
  - State.Running (status)
  - GraphDriver.Data["MergedDir"] (filesystem)
  - HostConfig.Runtime (runc/containerd)
- ContainerInfo struct at the bottom with mapped fields:
  - ID, Name, PID, State, RootFS, Runtime, BundlePath
  - Namespaces map (ipc, mnt, net, pid, user, uts, cgroup)
  - CgroupPath
- Show mapping arrows between JSON fields and struct fields
- Include Linux namespace icons (/proc/PID/ns/*)
- Use a structured layout with clear field mappings
```

## Diagram 4: CRIU Configuration and External Mounts

**Prompt for Gemini:**
```
Create a diagram showing CRIU configuration for Docker containers:
- Running Docker Container at the top with its filesystem layers
- CRIU Configuration block in the middle showing:
  - criuOpts structure with key settings:
    - Pid (target process)
    - LogLevel (verbose logging)
    - Root (container filesystem)
    - ManageCgroups (true)
    - ShellJob (true for docker)
    - LeaveRunning (configurable)
- External Mounts section showing Docker-managed filesystems:
  - /proc (process filesystem)
  - /dev (device filesystem)
  - /sys (system filesystem)
  - /etc/hostname, /etc/hosts, /etc/resolv.conf (Docker networking)
  - /sys/fs/cgroup (cgroup filesystem)
- Checkpoint Output at the bottom showing generated files:
  - core-*.img, pages-*.img, pagemap-*.img
  - fdinfo-*.img, mountpoints-*.img, netdev-*.img
  - container.json, dump.log
- Use different colors for internal vs external mounts
- Show the "external" designation preventing these mounts from being checkpointed
```

## Usage Instructions

1. Copy each prompt individually to Gemini
2. Ask Gemini to create the diagram as described
3. Request specific formats if needed (PNG, SVG, etc.)
4. You can ask for style modifications like:
   - "Make it more colorful"
   - "Use a minimalist style"
   - "Add more technical details"
   - "Make it suitable for documentation"

## Suggested Diagram Placement in README

- **Diagram 1** (System Architecture): After the "Introduction" section
- **Diagram 2** (Process Flow): In the "Architecture & Implementation" section
- **Diagram 3** (Container Info): In the "Go Code Implementation" subsection
- **Diagram 4** (CRIU Config): After the CRIU configuration code examples