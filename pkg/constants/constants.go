package constants

const (
	// Common Ignite prefix
	IGNITE_PREFIX = "ignite"

	// Ignite data base directory
	DATA_DIR = "/var/lib/firecracker"

	// Permissions for the data directory and its subdirectories
	DATA_DIR_PERM = 0755

	// Permissions for files in the data directory
	// TODO: Make all writes to DATA_DIR use this
	DATA_DIR_FILE_PERM = 644

	// Filename for metadata files
	METADATA = "metadata.json"

	// DHCP infinite lease time
	DHCP_INFINITE_LEASE = "4294967295s"

	// TAP adapter prefix in the parent container
	TAP_PREFIX = "vm_"

	// Bridge device prefix in the parent container
	BRIDGE_PREFIX = "br_"

	// Timeout in seconds to wait for VM shutdown before SIGKILL
	STOP_TIMEOUT = 20

	// Additional timeout in seconds for docker to wait for ignite to save and quit
	IGNITE_TIMEOUT = 10

	// In-container file name for the firecracker socket
	FIRECRACKER_API_SOCKET = "firecracker.sock"

	// In-container file name for the firecracker log FIFO
	LOG_FIFO = "firecracker_log.fifo"

	// In-container file name for the firecracker metrics FIFO
	METRICS_FIFO = "firecracker_metrics.fifo"

	// Persistent file to stream to from firecracker log FIFO
	LOG_FIFO_PERSISTENT = "firecracker_log.log"

	// Persistent file to stream to from firecracker metrics FIFO
	METRICS_FIFO_PERSISTENT = "firecracker_metrics.log"

	// Socket with a web server (with metrics for now) for the daemon
	DAEMON_SOCKET = "daemon.sock"

	// How many characters Ignite UIDs should have
	IGNITE_UID_LENGTH = 16

	// Max concurrent writers to LOG_FIFO and METRICS_FIFO
	MAX_CONCURRENT_WRITERS = 5
)
