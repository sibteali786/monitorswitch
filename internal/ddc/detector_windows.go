package ddc

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func (d *Detector) GetOSInfo() string {
	// TODO: Return a formatted string with OS info
	// Example: "Operating System: linux (Ubuntu 22.04)"
	switch d.osType {
	case OSWindows:
		info, err := d.DetectWindowsInfo()
		if err != nil {
			return fmt.Sprintf("Operating System: %s (Error: %v)", d.osType, err)
		}

		return fmt.Sprintf("Operating System: %s (%s %s)", d.osType, info.ProductName, info.Version)
	}
	return ""
}

// CreateDDCClient creates the appropriate DDC client for the current OS
func (d *Detector) CreateDDCClient() (DDCClient, error) {
	// TODO: Based on OS type, return appropriate client
	// For now, return nil and an error saying "not implemented"
	return nil, fmt.Errorf("DDC client not implemented for OS: %s", d.osType)
}
func (d *Detector) CheckDDCSupport() (bool, string) {
	switch d.osType {
	case OSWindows:
		return d.checkWindowsDDCSupport()
	}
	return false, "DDC support check not implemented"
}

func (d *Detector) checkWindowsDDCSupport() (bool, string) {
	if _, err := exec.LookPath("ddccci"); err == nil {
		return false, "DDC/CI support detected via ddccci"
	}

	// Check for ControlMyMonitor (NirSoft tool)
	if _, err := exec.LookPath("ControlMyMonitor"); err == nil {
		return true, "DDC/CI support detected via ControlMyMonitor"
	}

	return false, "No DDC/CI tools found. Install ddccci or ControlMyMonitor"
}

func (d *Detector) DetectMonitors() ([]Monitor, error) {
	if d.osType != OSWindows {
		return []Monitor{}, fmt.Errorf("not running on Windows")
	}
	// Check if any DDC tool is available
	if supported, _ := d.checkWindowsDDCSupport(); !supported {
		return []Monitor{}, fmt.Errorf("no DDC/CI tools available")
	}

	// Placeholder - will implement real detection later
	return []Monitor{}, nil
}

func (d *Detector) DetectWindowsInfo() (*WindowsInfo, error) {
	if d.osType != OSWindows {
		return nil, fmt.Errorf("not running on Windows")
	}

	info := &WindowsInfo{}

	if err := d.parseWindowsRegistry(info); err == nil {
		return info, nil
	}

	if err := d.parseSystemInfo(info); err == nil {
		return info, nil
	}

	if err := d.parseWMI(info); err == nil {
		return info, nil
	}

	if err := d.parseVerCommand(info); err == nil {
		return info, nil
	}

	return nil, fmt.Errorf("could not detect Windows system information")
}

func (d *Detector) parseWindowsRegistry(info *WindowsInfo) error {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)

	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}

	defer key.Close()

	if productName, _, err := key.GetStringValue("ProductName"); err == nil {
		info.ProductName = productName
	}
	// Current Version
	if version, _, err := key.GetStringValue("CurrentVersion"); err == nil {
		info.Version = version
	}

	// Current Build
	if build, _, err := key.GetStringValue("CurrentBuild"); err == nil {
		info.Build = build
	}

	// Display Version (Windows 10 20H1+)
	if displayVersion, _, err := key.GetStringValue("DisplayVersion"); err == nil {
		info.DisplayVersion = displayVersion
	}

	// Edition ID
	if editionID, _, err := key.GetStringValue("EditionID"); err == nil {
		info.Edition = editionID
	}

	// Install Date
	if installDate, _, err := key.GetIntegerValue("InstallDate"); err == nil {
		// Convert Unix timestamp to readable format
		info.InstallDate = fmt.Sprintf("%d", installDate)
	}

	// Registered Owner
	if owner, _, err := key.GetStringValue("RegisteredOwner"); err == nil {
		info.RegisteredOwner = owner
	}

	// System Root
	if systemRoot, _, err := key.GetStringValue("SystemRoot"); err == nil {
		info.SystemRoot = systemRoot
	}

	// Get processor architecture
	info.Architecture = d.getWindowsArchitecture()

	// Verify we got at least some information
	if info.ProductName == "" && info.Version == "" && info.Build == "" {
		return fmt.Errorf("no useful information found in registry")
	}

	return nil

}

func (d *Detector) parseSystemInfo(info *WindowsInfo) error {
	cmd := exec.Command("systeminfo")
	output, err := cmd.Output()

	if err != nil {
		return fmt.Errorf("systeminfo command failed: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "OS Name":
			info.ProductName = value
		case "OS Version":
			// Parse version like "10.0.22000 N/A Build 22000"
			if matches := regexp.MustCompile(`(\d+\.\d+\.\d+).*Build (\d+)`).FindStringSubmatch(value); len(matches) >= 3 {
				info.Version = matches[1]
				info.Build = matches[2]
			}
		case "System Type":
			info.Architecture = value
		case "Original Install Date":
			info.InstallDate = value
		case "Registered Owner":
			info.RegisteredOwner = value
		case "Windows Directory":
			info.SystemRoot = value
		}
	}

	// Verify we got at least some information
	if info.ProductName == "" && info.Version == "" {
		return fmt.Errorf("no useful information from systeminfo")
	}

	return nil
}

// parseWMI runs a WMI query and parses its output
func (d *Detector) parseWMI(info *WindowsInfo) error {
	cmd := exec.Command("wmic", "os", "get", "Caption,Version,BuildNumber,OSArchitecture", "/format:list")
	output, err := cmd.Output()

	if err != nil {
		return fmt.Errorf("wmic command failed: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Lines look like: "Caption=Microsoft Windows 11 Pro"
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Caption":
			info.ProductName = value
		case "Version":
			info.Version = value
		case "BuildNumber":
			info.Build = value
		case "OSArchitecture":
			info.Architecture = value
		}
	}

	// Verify we got at least some information
	if info.ProductName == "" && info.Version == "" {
		return fmt.Errorf("no useful information from WMI")
	}

	return nil
}

// parseVerCommand runs the "ver" command and parses its output
func (d *Detector) parseVerCommand(info *WindowsInfo) error {
	cmd := exec.Command("cmd", "/c", "ver")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ver command failed: %w", err)
	}

	line := strings.TrimSpace(string(output))
	if matches := regexp.MustCompile(`Microsoft Windows \[Version ([^\]]+)\]`).FindStringSubmatch(line); len(matches) >= 2 {
		info.Version = matches[1]
		info.ProductName = "Microsoft Windows"

		// Extract build number if present
		if versionParts := strings.Split(matches[1], "."); len(versionParts) >= 3 {
			info.Build = versionParts[2]
		}
	} else {
		return fmt.Errorf("could not parse ver command output")
	}

	info.Architecture = d.getWindowsArchitecture()

	return nil
}

func (d *Detector) getWindowsArchitecture() string {
	if arch := os.Getenv("PROCESSOR_ARCHITECTURE"); arch != "" {
		return arch
	}

	if arch := os.Getenv("PROCESSOR_ARCHITEW6432"); arch != "" {
		return arch
	}

	switch runtime.GOARCH {
	case "amd64":
		return "AMD64"
	case "386":
		return "x86"
	case "arm64":
		return "ARM64"
	case "arm":
		return "ARM"
	default:
		return runtime.GOARCH
	}
}
