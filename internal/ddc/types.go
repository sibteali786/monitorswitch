package ddc

import "runtime"

// OSType represents the operating system type
type OSType string

const (
	OSLinux   OSType = "linux"
	OSMacOS   OSType = "darwin"
	OSWindows OSType = "windows"
)

// LinuxInfo contains detailed Linux distribution information
type LinuxInfo struct {
	Name          string // Distribution name (e.g., "Ubuntu")
	Version       string // Version number (e.g., "20.04")
	ID            string // Distribution ID (e.g., "ubuntu")
	VersionID     string // Version ID (e.g., "20.04")
	PrettyName    string // Pretty name (e.g., "Ubuntu 20.04.3 LTS")
	Codename      string // Release codename (e.g., "focal")
	KernelName    string // Kernel name (e.g., "Linux")
	KernelRelease string // Kernel release (e.g., "5.4.0-88-generic")
	KernelVersion string // Kernel version
	Machine       string // Machine architecture (e.g., "x86_64")
}

// MacOSInfo contains detailed macOS system information
type MacOSInfo struct {
	ProductName    string // Product name (e.g., "macOS")
	ProductVersion string // Version (e.g., "12.6")
	BuildVersion   string // Build version (e.g., "21G115")
	KernelName     string // Kernel name (e.g., "Darwin")
	KernelRelease  string // Kernel release (e.g., "21.6.0")
	KernelVersion  string // Kernel version
	Machine        string // Machine architecture (e.g., "x86_64")
	ModelName      string // Model name (e.g., "MacBook Pro")
	ModelID        string // Model identifier (e.g., "MacBookPro16,1")
}

// WindowsInfo contains detailed Windows system information
type WindowsInfo struct {
	ProductName     string // Product name (e.g., "Windows 11 Pro")
	Version         string // Version (e.g., "10.0.22000")
	Build           string // Build number (e.g., "22000")
	DisplayVersion  string // Display version (e.g., "21H2")
	Edition         string // Edition (e.g., "Pro", "Home")
	Architecture    string // Architecture (e.g., "AMD64")
	InstallDate     string // Install date
	RegisteredOwner string // Registered owner
	SystemRoot      string // System root (e.g., "C:\\Windows")
}

// DDCClient interface defines the contract for DDC/CI monitor control
type DDCClient interface {
	DetectMonitors() ([]Monitor, error)
	GetCapabilities(monitorId string) (*Capabilities, error)
	SetVCP(monitorID string, code byte, value uint16) error
	GetVCP(monitorID string, code byte) (uint16, error)
}

// Monitor represents a physical monitor
type Monitor struct {
	ID           string          // Unique monitor identifier
	Name         string          // Human-readable monitor name
	Inputs       map[string]byte // Available input sources (name -> VCP code)
	CurrentInput string          // Currently active input source
}

// Capabilities represents monitor capabilities
type Capabilities struct {
	SupportedInputs     map[string]byte // Supported input sources (name -> VCP code)
	SupportedBrightness bool            // Whether brightness control is supported
	SupportedContrast   bool            // Whether contrast control is supported
}

// Detector is the main OS detection struct
type Detector struct {
	osType OSType
}

// NewDetector creates a new OS detector instance
func NewDetector() *Detector {
	return &Detector{
		osType: OSType(runtime.GOOS),
	}
}

// GetOSType returns the current operating system type
func (d *Detector) GetOSType() OSType {
	return d.osType
}
