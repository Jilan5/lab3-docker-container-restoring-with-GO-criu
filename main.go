package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	criu "github.com/checkpoint-restore/go-criu/v7"
	"github.com/checkpoint-restore/go-criu/v7/rpc"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"google.golang.org/protobuf/proto"
)

// ContainerInfo holds essential container details for checkpointing
type ContainerInfo struct {
	ID         string
	Name       string
	PID        int
	State      string
	RootFS     string
	Runtime    string
	BundlePath string
	Namespaces map[string]string
	CgroupPath string
}

// Options for checkpoint operation
type Options struct {
	LeaveRunning   bool
	TCPEstablished bool
	FileLocks      bool
	PreDump        bool
}

func main() {
	var (
		containerName  string
		checkpointName string
		baseDir        string
		leaveRunning   bool
		tcpEstablished bool
		fileLocks      bool
		preDump        bool
		restore        bool
		newName        string
	)

	flag.StringVar(&containerName, "container", "", "Container name or ID to checkpoint/restore")
	flag.StringVar(&checkpointName, "name", "checkpoint1", "Name for the checkpoint")
	flag.StringVar(&baseDir, "dir", "/tmp/docker-checkpoints", "Base directory for checkpoints")
	flag.BoolVar(&leaveRunning, "leave-running", true, "Leave container running after checkpoint")
	flag.BoolVar(&tcpEstablished, "tcp", true, "Checkpoint established TCP connections")
	flag.BoolVar(&fileLocks, "file-locks", true, "Checkpoint file locks")
	flag.BoolVar(&preDump, "pre-dump", false, "Perform pre-dump for optimization")
	flag.BoolVar(&restore, "restore", false, "Restore container from checkpoint")
	flag.StringVar(&newName, "new-name", "", "New name for restored container (optional)")

	flag.Parse()

	if containerName == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -container <name> [options]\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	if restore {
		// Restore mode
		fmt.Printf("Starting restore of container '%s' from checkpoint '%s'...\n", containerName, checkpointName)

		if newName == "" {
			newName = fmt.Sprintf("%s-restored", containerName)
		}

		if err := restoreContainer(containerName, checkpointName, baseDir, newName); err != nil {
			log.Fatal("Restore failed:", err)
		}

		fmt.Printf("\nContainer restored successfully as '%s'\n", newName)

		// Verify restoration
		if err := verifyRestoration(newName); err != nil {
			fmt.Printf("Warning: Restoration verification failed: %v\n", err)
		} else {
			fmt.Println("Restoration verified successfully!")
		}
	} else {
		// Checkpoint mode
		opts := Options{
			LeaveRunning:   leaveRunning,
			TCPEstablished: tcpEstablished,
			FileLocks:      fileLocks,
			PreDump:        preDump,
		}

		fmt.Printf("Starting checkpoint of container '%s'...\n", containerName)
		if err := checkpointContainer(containerName, checkpointName, baseDir, opts); err != nil {
			log.Fatal("Checkpoint failed:", err)
		}

		fmt.Printf("\nCheckpoint stored in: %s/%s/%s\n", baseDir, containerName, checkpointName)
		fmt.Println("\nCheckpoint contents:")

		checkpointPath := fmt.Sprintf("%s/%s/%s", baseDir, containerName, checkpointName)
		files, _ := os.ReadDir(checkpointPath)
		for _, file := range files {
			info, _ := file.Info()
			fmt.Printf("  %s (%d bytes)\n", file.Name(), info.Size())
		}
	}
}

func checkpointContainer(containerName, checkpointName, baseDir string, opts Options) error {
	// Get container information
	info, err := inspectContainer(containerName)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	// Print container info
	printContainerInfo(info)

	// Create checkpoint directory
	checkpointDir := filepath.Join(baseDir, info.Name, checkpointName)
	if err := os.MkdirAll(checkpointDir, 0755); err != nil {
		return fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	fmt.Printf("\nCheckpointing to: %s\n", checkpointDir)

	// Perform the checkpoint
	if err := doCRIUCheckpoint(info, checkpointDir, opts); err != nil {
		return fmt.Errorf("checkpoint failed: %w", err)
	}

	// Save metadata
	if err := saveMetadata(info, checkpointDir); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	fmt.Printf("Checkpoint successful!\n")
	return nil
}

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

	runtime := containerJSON.HostConfig.Runtime
	if runtime == "" {
		runtime = "runc"
	}

	info := &ContainerInfo{
		ID:         containerJSON.ID[:12],
		Name:       strings.TrimPrefix(containerJSON.Name, "/"),
		PID:        containerJSON.State.Pid,
		State:      containerJSON.State.Status,
		RootFS:     containerJSON.GraphDriver.Data["MergedDir"],
		Runtime:    runtime,
		BundlePath: fmt.Sprintf("/run/docker/runtime-%s/moby/%s", runtime, containerJSON.ID),
		CgroupPath: containerJSON.HostConfig.CgroupParent,
		Namespaces: make(map[string]string),
	}

	// Get namespace information
	nsTypes := []string{"ipc", "mnt", "net", "pid", "user", "uts", "cgroup"}
	for _, ns := range nsTypes {
		info.Namespaces[ns] = fmt.Sprintf("/proc/%d/ns/%s", info.PID, ns)
	}

	return info, nil
}

func doCRIUCheckpoint(info *ContainerInfo, checkpointDir string, opts Options) error {
	criuClient := criu.MakeCriu()
	criuClient.SetCriuPath("criu")

	cgroupPath := info.CgroupPath
	if cgroupPath == "" {
		cgroupPath = fmt.Sprintf("/docker/%s", info.ID)
	}

	criuOpts := &rpc.CriuOpts{
		Pid:            proto.Int32(int32(info.PID)),
		LogLevel:       proto.Int32(4),
		LogFile:        proto.String("dump.log"),
		Root:           proto.String(info.RootFS),
		ManageCgroups:  proto.Bool(true),
		TcpEstablished: proto.Bool(opts.TCPEstablished),
		FileLocks:      proto.Bool(opts.FileLocks),
		LeaveRunning:   proto.Bool(opts.LeaveRunning),
		External: []string{
			"mnt[/proc]:proc",
			"mnt[/dev]:dev",
			"mnt[/sys]:sys",
			"mnt[/dev/shm]:shm",
			"mnt[/dev/pts]:pts",
			"mnt[/dev/mqueue]:mqueue",
			"mnt[/etc/hostname]:hostname",
			"mnt[/etc/hosts]:hosts",
			"mnt[/etc/resolv.conf]:resolv.conf",
			"mnt[/sys/fs/cgroup]:cgroup",
		},
		ShellJob: proto.Bool(true),
		CgRoot: []*rpc.CgroupRoot{
			{
				Ctrl: proto.String("cpu"),
				Path: proto.String(cgroupPath),
			},
			{
				Ctrl: proto.String("memory"),
				Path: proto.String(cgroupPath),
			},
		},
	}

	workDir, err := os.Open(checkpointDir)
	if err != nil {
		return fmt.Errorf("failed to open checkpoint directory: %w", err)
	}
	defer workDir.Close()

	// Set images directory using file descriptor
	criuOpts.ImagesDirFd = proto.Int32(int32(workDir.Fd()))

	if opts.PreDump {
		fmt.Println("Performing pre-dump...")
		preDumpOpts := *criuOpts
		preDumpOpts.TrackMem = proto.Bool(true)
		preDumpOpts.TcpEstablished = proto.Bool(false)

		if err := criuClient.PreDump(&preDumpOpts, nil); err != nil {
			return fmt.Errorf("pre-dump failed: %w", err)
		}
	}

	fmt.Println("Performing checkpoint...")

	if err := criuClient.Dump(criuOpts, nil); err != nil {
		logPath := filepath.Join(checkpointDir, "dump.log")
		if logData, readErr := os.ReadFile(logPath); readErr == nil {
			fmt.Printf("CRIU log:\n%s\n", logData)
		}
		return fmt.Errorf("CRIU dump failed: %w", err)
	}

	return nil
}

func saveMetadata(info *ContainerInfo, checkpointDir string) error {
	metadataFile := filepath.Join(checkpointDir, "container.json")

	metadata := map[string]interface{}{
		"id":          info.ID,
		"name":        info.Name,
		"runtime":     info.Runtime,
		"rootfs":      info.RootFS,
		"bundle_path": info.BundlePath,
		"namespaces":  info.Namespaces,
		"cgroup_path": info.CgroupPath,
	}

	file, err := os.Create(metadataFile)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(metadata)
}

func printContainerInfo(info *ContainerInfo) {
	fmt.Printf("Container Information:\n")
	fmt.Printf("  ID:         %s\n", info.ID)
	fmt.Printf("  Name:       %s\n", info.Name)
	fmt.Printf("  PID:        %d\n", info.PID)
	fmt.Printf("  State:      %s\n", info.State)
	fmt.Printf("  Runtime:    %s\n", info.Runtime)
	fmt.Printf("  RootFS:     %s\n", info.RootFS)
	fmt.Printf("  Bundle:     %s\n", info.BundlePath)
	fmt.Printf("  Cgroup:     %s\n", info.CgroupPath)
	fmt.Printf("  Namespaces:\n")
	for ns, path := range info.Namespaces {
		fmt.Printf("    %s: %s\n", ns, path)
	}
}

func restoreContainer(originalName, checkpointName, baseDir, newName string) error {
	checkpointDir := filepath.Join(baseDir, originalName, checkpointName)

	// Check if checkpoint exists
	if _, err := os.Stat(checkpointDir); os.IsNotExist(err) {
		return fmt.Errorf("checkpoint does not exist at %s", checkpointDir)
	}

	// Load metadata
	metadataFile := filepath.Join(checkpointDir, "container.json")
	metadataBytes, err := os.ReadFile(metadataFile)
	if err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return fmt.Errorf("failed to parse metadata: %w", err)
	}

	fmt.Printf("Restoring from checkpoint at: %s\n", checkpointDir)
	fmt.Printf("Original container ID: %s\n", metadata["id"])
	fmt.Printf("New container name: %s\n", newName)

	// First, we need to create a new container in stopped state
	if err := createContainerForRestore(originalName, newName); err != nil {
		return fmt.Errorf("failed to create container for restore: %w", err)
	}

	// Get the new container's info
	newInfo, err := inspectContainer(newName)
	if err != nil {
		// If container is not running, we need to get info differently
		newInfo, err = getStoppedContainerInfo(newName)
		if err != nil {
			return fmt.Errorf("failed to get new container info: %w", err)
		}
	}

	// Perform CRIU restore
	if err := doCRIURestore(newInfo, checkpointDir); err != nil {
		return fmt.Errorf("CRIU restore failed: %w", err)
	}

	return nil
}

func createContainerForRestore(originalName, newName string) error {
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}

	// Get original container's configuration
	originalInspect, err := cli.ContainerInspect(ctx, originalName)
	if err != nil {
		// If original doesn't exist, try to use a basic config
		fmt.Printf("Warning: Original container not found, using basic configuration\n")
		return createBasicContainer(cli, ctx, newName)
	}

	// Create new container with same configuration but don't start it
	config := originalInspect.Config
	hostConfig := originalInspect.HostConfig

	// Update the container name
	config.Hostname = newName

	// Create the container (but don't start it)
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, newName)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	fmt.Printf("Created container with ID: %s\n", resp.ID[:12])
	return nil
}

func createBasicContainer(cli *client.Client, ctx context.Context, name string) error {
	// Create a basic alpine container that we can restore into
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"sleep", "infinity"},
	}, &container.HostConfig{
		Privileged: true,
		PidMode:    "host",
	}, nil, nil, name)

	if err != nil {
		return fmt.Errorf("failed to create basic container: %w", err)
	}

	fmt.Printf("Created basic container with ID: %s\n", resp.ID[:12])
	return nil
}

func getStoppedContainerInfo(containerName string) (*ContainerInfo, error) {
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	containerJSON, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	runtime := containerJSON.HostConfig.Runtime
	if runtime == "" {
		runtime = "runc"
	}

	// For stopped container, PID will be 0
	info := &ContainerInfo{
		ID:         containerJSON.ID[:12],
		Name:       strings.TrimPrefix(containerJSON.Name, "/"),
		PID:        0, // Will be set during restore
		State:      containerJSON.State.Status,
		RootFS:     containerJSON.GraphDriver.Data["MergedDir"],
		Runtime:    runtime,
		BundlePath: fmt.Sprintf("/run/docker/runtime-%s/moby/%s", runtime, containerJSON.ID),
		CgroupPath: containerJSON.HostConfig.CgroupParent,
		Namespaces: make(map[string]string),
	}

	return info, nil
}

func doCRIURestore(info *ContainerInfo, checkpointDir string) error {
	criuClient := criu.MakeCriu()
	criuClient.SetCriuPath("criu")

	cgroupPath := info.CgroupPath
	if cgroupPath == "" {
		cgroupPath = fmt.Sprintf("/docker/%s", info.ID)
	}

	criuOpts := &rpc.CriuOpts{
		LogLevel:       proto.Int32(4),
		LogFile:        proto.String("restore.log"),
		Root:           proto.String(info.RootFS),
		ManageCgroups:  proto.Bool(true),
		TcpEstablished: proto.Bool(true),
		FileLocks:      proto.Bool(true),
		External: []string{
			"mnt[/proc]:proc",
			"mnt[/dev]:dev",
			"mnt[/sys]:sys",
			"mnt[/dev/shm]:shm",
			"mnt[/dev/pts]:pts",
			"mnt[/dev/mqueue]:mqueue",
			"mnt[/etc/hostname]:hostname",
			"mnt[/etc/hosts]:hosts",
			"mnt[/etc/resolv.conf]:resolv.conf",
			"mnt[/sys/fs/cgroup]:cgroup",
		},
		ShellJob:       proto.Bool(true),
		RstSibling:     proto.Bool(true),
		RestoreDetached: proto.Bool(true),
		CgRoot: []*rpc.CgroupRoot{
			{
				Ctrl: proto.String("cpu"),
				Path: proto.String(cgroupPath),
			},
			{
				Ctrl: proto.String("memory"),
				Path: proto.String(cgroupPath),
			},
		},
	}

	workDir, err := os.Open(checkpointDir)
	if err != nil {
		return fmt.Errorf("failed to open checkpoint directory: %w", err)
	}
	defer workDir.Close()

	// Set images directory using file descriptor
	criuOpts.ImagesDirFd = proto.Int32(int32(workDir.Fd()))

	fmt.Println("Performing restore...")

	if err := criuClient.Restore(criuOpts, nil); err != nil {
		logPath := filepath.Join(checkpointDir, "restore.log")
		if logData, readErr := os.ReadFile(logPath); readErr == nil {
			fmt.Printf("CRIU restore log:\n%s\n", logData)
		}
		return fmt.Errorf("CRIU restore failed: %w", err)
	}

	return nil
}

func verifyRestoration(containerName string) error {
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}

	// Check container status
	containerJSON, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("failed to inspect restored container: %w", err)
	}

	fmt.Printf("\nRestored Container Status:\n")
	fmt.Printf("  Name:       %s\n", strings.TrimPrefix(containerJSON.Name, "/"))
	fmt.Printf("  ID:         %s\n", containerJSON.ID[:12])
	fmt.Printf("  State:      %s\n", containerJSON.State.Status)
	fmt.Printf("  Running:    %v\n", containerJSON.State.Running)

	if containerJSON.State.Running {
		fmt.Printf("  PID:        %d\n", containerJSON.State.Pid)
		fmt.Printf("  Started At: %s\n", containerJSON.State.StartedAt)

		// Try to get container logs to verify it's working
		options := container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "10",
		}

		logs, err := cli.ContainerLogs(ctx, containerName, options)
		if err == nil {
			defer logs.Close()
			fmt.Printf("\nRecent container logs:\n")
			buf := make([]byte, 1024)
			n, _ := logs.Read(buf)
			if n > 0 {
				fmt.Printf("%s\n", buf[:n])
			}
		}

		// Get container stats to verify it's actually running
		stats, err := cli.ContainerStats(ctx, containerName, false)
		if err == nil {
			defer stats.Body.Close()
			fmt.Printf("\n✓ Container is responding to API calls\n")
		}

		return nil
	}

	return fmt.Errorf("container is not running after restore")
}