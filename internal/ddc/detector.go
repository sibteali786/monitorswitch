//go:build !windows
// +build !windows

package ddc

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"
)

func (d *Detector) GetOSInfo() string {
	// TODO: Return a formatted string with OS info
	// Example: "Operating System: linux (Ubuntu 22.04)"
	switch d.osType {
	case OSLinux:
		info, err := d.DetectLinuxInfo()
		if err != nil {
			return fmt.Sprintf("Operating System: %s (Error: %v)", d.osType, err)
		}
		return fmt.Sprintf("Operating System: %s (%s %s)", d.osType, info.Name, info.Version)
	case OSMacOS:
		info, err := d.DetectMacOSInfo()
		if err != nil {
			return fmt.Sprintf("Operating System: %s (Error: %v)", d.osType, err)
		}
		return fmt.Sprintf("Operating System: %s (%s %s)", d.osType, info.ProductName, info.ProductVersion)
	}
	return ""
}

// CreateDDCClient creates the appropriate DDC client for the current OS
func (d *Detector) CreateDDCClient() (DDCClient, error) {
	// TODO: Based on OS type, return appropriate client
	// For now, return nil and an error saying "not implemented"
	return nil, fmt.Errorf("DDC client not implemented for OS: %s", d.osType)
}

// CheckDDCSupport checks if DDC/CI is supported on current system
func (d *Detector) CheckDDCSupport() (bool, string) {
	// TODO: Check if required tools are available
	// Linux: check for ddcutil
	// macOS: check for m1ddc or ddcctl
	// Windows: check for ddccci or similar
	// Return (supported, message)
	switch d.osType {
	case OSLinux:
		if _, err := exec.LookPath("ddcutil"); err == nil {
			return true, "DDC/CI support detected via ddcutil"
		} else {
			return false, "ddcutil not found, DDC/CI support may not be available"
		}
	case OSMacOS:
		if _, err := exec.LookPath("m1ddc"); err == nil {
			return true, "DDC/CI support detected via m1ddc or ddcctl"
		} else if _, err := exec.LookPath("ddcctl"); err == nil {
			return true, "DDC/CI support detected via m1ddc or ddcctl"
		}
	}
	return false, "DDC support check not implemented"
}

func (d *Detector) DetectLinuxInfo() (*LinuxInfo, error) {
	if d.osType != OSLinux {
		return nil, fmt.Errorf("not running on Linux")
	}

	info := &LinuxInfo{}

	if err := d.getKernelInfo(info); err != nil {
		fmt.Printf("Warning: could not get kernel info: %v\n", err)
	}

	// Try to get distribution info from various sources
	if err := d.getDistributionInfo(info); err != nil {
		return nil, fmt.Errorf("failed to detect distribution info: %w", err)
	}

	return info, nil
}

// getKernelInfo uses syscall to get kernel information
func (d *Detector) getKernelInfo(info *LinuxInfo) error {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		return err
	}

	info.KernelName = string(utsname.Sysname[:clen(utsname.Sysname[:])])
	info.KernelRelease = string(utsname.Release[:clen(utsname.Release[:])])
	info.KernelVersion = string(utsname.Version[:clen(utsname.Version[:])])
	info.Machine = string(utsname.Machine[:clen(utsname.Machine[:])])

	return nil
}

// getDistributionInfo tries multiple sources to get distribution information

func (d *Detector) getDistributionInfo(info *LinuxInfo) error {
	// Try /etc/os-release first (modern standard)
	if err := d.parseOSRelease(info); err == nil {
		return nil
	}

	// Try /etc/lsb-release (LSB standard)
	if err := d.parseLSBRelease(info); err == nil {
		return nil
	}

	// Try distribution-specific files
	if err := d.parseDistributionSpecificFiles(info); err == nil {
		return nil
	}

	return fmt.Errorf("could not detect distribution information")

}

func (d *Detector) parseOSRelease(info *LinuxInfo) error {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)

		switch key {
		case "NAME":
			info.Name = value
		case "VERSION":
			info.Version = value
		case "ID":
			info.ID = value
		case "VERSION_ID":
			info.VersionID = value
		case "PRETTY_NAME":
			info.PrettyName = value
		case "VERSION_CODENAME":
			info.Codename = value
		case "UBUNTU_CODENAME": // Ubuntu specific
			if info.Codename == "" {
				info.Codename = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Verify we got at least some information
	if info.Name == "" && info.ID == "" && info.PrettyName == "" {
		return fmt.Errorf("no useful information found in /etc/os-release")
	}

	return nil
}

func (d *Detector) parseLSBRelease(info *LinuxInfo) error {
	file, err := os.Open("/etc/lsb-release")

	if err != nil {
		return err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// because line we are interested look like DISTRIB_ID=Ubuntu
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		switch key {
		case "DISTRIB_ID":
			info.ID = strings.ToLower(value)
			info.Name = value
		case "DISTRIB_RELEASE":
			info.Version = value
			info.VersionID = value
		case "DISTRIB_DESCRIPTION":
			info.PrettyName = value
		case "DISTRIB_CODENAME":
			info.Codename = value
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if info.Name == "" && info.ID == "" {
		return fmt.Errorf("no useful information found in /etc/lsb-release")
	}

	return nil

}

func (d *Detector) parseDistributionSpecificFiles(info *LinuxInfo) error {
	releaseFiles := []struct {
		path   string
		distro string
	}{
		{"/etc/redhat-release", "redhat"},
		{"/etc/centos-release", "centos"},
		{"/etc/fedora-release", "fedora"},
		{"/etc/debian_version", "debian"},
		{"/etc/arch-release", "arch"},
		{"/etc/gentoo-release", "gentoo"},
		{"/etc/alpine-release", "alpine"},
		{"/etc/slackware-version", "slackware"},
	}

	for _, rf := range releaseFiles {
		if content, err := d.readReleaseFile(rf.path); err == nil {
			info.ID = rf.distro
			info.Name = strings.Title(rf.distro)
			info.PrettyName = content

			// Try to extract version from content
			if version := extractVersion(content); version != "" {
				info.Version = version
				info.VersionID = version
			}

			return nil

		}
	}
	return fmt.Errorf("no distribution-specific files found")
}

// readReleaseFile reads the content of a release file
func (d Detector) readReleaseFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("empty file")
}

func extractVersion(content string) string {
	// Simple version extraction - looks for patterns like "8.5", "20.04", etc.
	words := strings.Fields(content)
	for _, word := range words {
		if strings.Contains(word, ".") {
			// Check if it looks like a version number
			if len(word) >= 3 && (word[0] >= '0' && word[0] <= '9') {
				return word
			}
		}
	}
	return ""
}
func clen(b []byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == 0 {
			return i
		}
	}
	return len(b)
}

/* Macos Detailed Info Detection */
func (d *Detector) DetectMacOSInfo() (*MacOSInfo, error) {
	if d.osType != OSMacOS {
		return nil, fmt.Errorf("not running on macOS")
	}

	info := &MacOSInfo{}

	// Get system information using sysctl
	if err := d.getMacOSSystemInfo(info); err != nil {
		fmt.Printf("Warning: could not get kernel info: %v\n", err)
	}

	if err := d.getMacOSSystemInfo(info); err != nil {
		return nil, fmt.Errorf("failed to detect macOS system info: %w", err)
	}
	return info, nil
}

func (d *Detector) getMacOSSystemInfo(info *MacOSInfo) error {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		return err
	}

	info.KernelName = string(utsname.Sysname[:clen(utsname.Sysname[:])])
	info.KernelRelease = string(utsname.Release[:clen(utsname.Release[:])])
	info.KernelVersion = string(utsname.Version[:clen(utsname.Version[:])])
	info.Machine = string(utsname.Machine[:clen(utsname.Machine[:])])

	return nil
}

func (d *Detector) GetMacOSSystemInfo(info *MacOSInfo) error {
	if err := d.parseSWVers(info); err == nil {
		d.getMacOSHardwareInfo(info)
		return nil
	}

	if err := d.parseSystemVersionPlist(info); err == nil {
		d.getMacOSHardwareInfo(info)
		return nil
	}

	return fmt.Errorf("could not detect macOS system information")
}

func (d *Detector) parseSWVers(info *MacOSInfo) error {
	cmd := exec.Command("sw_vers")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed")
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
		case "ProductName":
			info.ProductName = value
		case "ProductVersion":
			info.ProductVersion = value
		case "BuildVersion":
			info.BuildVersion = value
		}
	}

	// Verify we got at least some information
	if info.ProductName == "" && info.ProductVersion == "" {
		return fmt.Errorf("no useful information from sw_vers")
	}

	return nil

}

type SystemVersionPlist struct {
	XMLName xml.Name `xml:"plist"`
	Dict    Dict     `xml:"dict"`
}

// because xml looks like this
// <dict>
//         <key>BuildID</key>
//         <string>BB647568-6456-11F0-B4BA-837B213D850F</string>
//         <key>ProductBuildVersion</key>
//         <string>24G84</string>
//         <key>ProductCopyright</key>
//         <string>1983-2025 Apple Inc.</string>
//         <key>ProductName</key>
//         <string>macOS</string>
//         <key>ProductUserVisibleVersion</key>
//         <string>15.6</string>
//         <key>ProductVersion</key>
//         <string>15.6</string>
//         <key>iOSSupportVersion</key>
//         <string>18.6</string>
// </dict>

type Dict struct {
	Keys   []string `xml:"key"`
	Values []string `xml:"string"`
}

// its called xml unmarshaling or XML deserialization

// parseSystemVersionPlist parses /System/Library/CoreServices/SystemVersion.plist
func (d *Detector) parseSystemVersionPlist(info *MacOSInfo) error {
	plistPath := "/System/Library/CoreServices/SystemVersion.plist"
	file, err := os.Open(plistPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", plistPath, err)
	}

	defer file.Close()

	var plist SystemVersionPlist
	decoder := xml.NewDecoder(file)
	if err := decoder.Decode(&plist); err != nil {
		return fmt.Errorf("failed to parse SystemVersion.plist: %w", err)
	}

	dict := plist.Dict
	if len(dict.Keys) != len(dict.Values) {
		return fmt.Errorf("malformed plist: keys and values count mismatch")
	}

	for i, key := range dict.Keys {
		if i >= len(dict.Values) {
			break
		}

		value := dict.Values[i]
		switch key {
		case "ProductName":
			info.ProductName = value
		case "ProductVersion":
			info.ProductVersion = value
		case "ProductBuildVersion":
			info.BuildVersion = value
		}

	}

	// Verify we got at least some information
	if info.ProductName == "" && info.ProductVersion == "" {
		return fmt.Errorf("no useful information from SystemVersion.plist")
	}

	return nil
}

func (d *Detector) getMacOSHardwareInfo(info *MacOSInfo) {
	// Try to get model information using system_profiler
	if err := d.parseSystemProfiler(info); err != nil {
		// Try to get model from sysctl as fallback
		d.parseSystemctl(info)
	}
}

func (d *Detector) parseSystemProfiler(info *MacOSInfo) error {
	cmd := exec.Command("system_profiler", "SPHardwareDataType")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("system_profiler command failed: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for lines like "Model Name: MacBook Pro"
		if strings.Contains(line, "Model Name:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				info.ModelName = strings.TrimSpace(parts[1])
			}
		}

		// Look for lines like "Model Identifier: MacBookPro16,1"
		if strings.Contains(line, "Model Identifier:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				info.ModelID = strings.TrimSpace(parts[1])
			}
		}
	}
	return nil
}

// parseSystemctl uses sysctl to get model information
func (d *Detector) parseSystemctl(info *MacOSInfo) {
	if cmd := exec.Command("sysctl", "-n", "hw.model"); cmd != nil {
		if output, err := cmd.Output(); err == nil {
			info.ModelName = strings.TrimSpace(string(output))
		}
	}
}

// Monitor detection methods
func (d *Detector) DetectMonitors() ([]Monitor, error) {
	switch d.osType {
	case OSLinux:
		return d.detectLinuxMonitors()
	case OSMacOS:
		return d.detectMacOSMonitors()
	default:
		return []Monitor{}, fmt.Errorf("monitor detection not implemented for OS: %s", d.osType)
	}
}

func (d *Detector) detectLinuxMonitors() ([]Monitor, error) {
	if _, err := exec.LookPath("ddcutil"); err != nil {
		return []Monitor{}, fmt.Errorf("ddcutil not found: %v", err)
	}

	cmd := exec.Command("ddcutil", "detect")
	output, err := cmd.Output()
	if err != nil {
		return []Monitor{}, fmt.Errorf("ddcutil detect failed: %v", err)
	}
	if len(strings.TrimSpace(string(output))) == 0 {
		return []Monitor{}, nil
	}

	// Placeholder - will implement real parsing later
	return []Monitor{}, nil
}

func (d *Detector) detectMacOSMonitors() ([]Monitor, error) {
	// Check for m1ddc first
	if _, err := exec.LookPath("m1ddc"); err == nil {
		return d.detectWithM1DDC()
	}

	// Check for ddcctl
	if _, err := exec.LookPath("ddcctl"); err == nil {
		return d.detectWithDDCCTL()
	}

	return []Monitor{}, fmt.Errorf("neither m1ddc nor ddcctl found")
}

func (d *Detector) detectWithM1DDC() ([]Monitor, error) {
	cmd := exec.Command("m1ddc", "display", "list")
	output, err := cmd.Output()
	if err != nil {
		return []Monitor{}, fmt.Errorf("m1ddc list failed: %w", err)
	}

	// For now, return empty if no output
	if len(strings.TrimSpace(string(output))) == 0 {
		return []Monitor{}, nil
	}

	// Placeholder - will implement real parsing later
	return []Monitor{}, nil
}

func (d *Detector) detectWithDDCCTL() ([]Monitor, error) {
	cmd := exec.Command("ddcctl", "-d", "1")
	_, err := cmd.Output()
	if err != nil {
		// If display 1 doesn't exist, no monitors
		return []Monitor{}, nil
	}

	// Placeholder - will implement real parsing later
	return []Monitor{}, nil
}
