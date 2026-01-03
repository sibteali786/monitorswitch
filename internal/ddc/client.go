package ddc

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DDCClientImpl implements the DDCClient interface for real DDC communication
type DDCClientImpl struct {
	osType OSType
}

var M1DDCInputSources = map[string]int{
	"DisplayPort": 15,
	"DP":          15,
	"DP-1":        15,
	"DP-2":        16,
	"HDMI":        17,
	"HDMI-1":      17,
	"HDMI-2":      18,
	"USB-C":       27,
	"Thunderbolt": 27,
}

// Corrected input source mappings for ddcctl (uses 1-18 range)
var DDCCTLInputSources = map[string]int{
	"HDMI-1":      17,
	"HDMI-2":      18,
	"DisplayPort": 15,
	"DP":          15,
	"USB-C":       27, // Not sure if ddcctl supports this, but we can try
}

type EnhancedMonitor struct {
	Monitor
	DDCSupported    bool            // Whether DDC commands work
	SupportedInputs map[string]byte // Detected input sources
	InputNames      []string        // Human-readable input names
	DDCTool         string          // "ddcctl", "m1ddc", or ""
}

type DDCValidationResult struct {
	ToolAvailable     bool
	CanReadValues     bool
	CanWriteValues    bool
	ValidationError   error
	RecommendedAction string
}

func NewDDCClientImpl(osType OSType) *DDCClientImpl {
	return &DDCClientImpl{
		osType: osType,
	}
}

// Detect all DDC-compatible monitors
func (c *DDCClientImpl) DetectMonitors() ([]Monitor, error) {
	switch c.osType {
	case OSLinux:
		return c.detectLinuxMonitors()
	case OSMacOS:
		return c.detectMacOSMonitors()
	case OSWindows:
		return c.detectWindowsMonitors()
	default:
		return nil, fmt.Errorf("unsupported OS: %s", c.osType)
	}
}

func (c *DDCClientImpl) GetCapabilities(monitorID string) (*Capabilities, error) {
	switch c.osType {
	case OSLinux:
		return c.getLinuxCapabilities(monitorID)
	case OSMacOS:
		return c.getMacOSCapabilities(monitorID)
	case OSWindows:
		return c.getWindowsCapabilities(monitorID)
	default:
		return nil, fmt.Errorf("unsupported OS: %s", c.osType)
	}
}

// SetVCP sets a VCP feature value (e.g., switch input, set brightness)
func (c *DDCClientImpl) SetVCP(monitorID string, code byte, value uint16) error {
	switch c.osType {
	case OSLinux:
		return c.setLinuxVCP(monitorID, code, value)
	case OSMacOS:
		return c.setMacOSVCP(monitorID, code, value)
	case OSWindows:
		return c.setWindowsVCP(monitorID, code, value)
	default:
		return fmt.Errorf("unsupported OS: %s", c.osType)
	}
}

func (c *DDCClientImpl) GetVCP(monitorID string, code byte) (uint16, error) {
	switch c.osType {
	case OSLinux:
		return c.getLinuxVCP(monitorID, code)
	case OSMacOS:
		return c.getMacOSVCP(monitorID, code)
	case OSWindows:
		return c.getWindowsVCP(monitorID, code)
	default:
		return 0, fmt.Errorf("unsupported OS: %s", c.osType)
	}
}

// ============ LINUX IMPLEMENTATION ============

func (c *DDCClientImpl) detectLinuxMonitors() ([]Monitor, error) {
	if monitors := c.detectWithCLITools(); len(monitors) > 0 {
		return monitors, nil
	}

	return c.detectWithCoreSystem()
}

func (c *DDCClientImpl) detectWithCLITools() []Monitor {
	if tool := c.detectAvailableDDCToolsLinux(); tool != "" {
		switch tool {
		case "ddcutil":
			if monitors := c.detectWithDdcutil(); len(monitors) > 0 {
				return monitors
			}
		}
	}

	return []Monitor{}

}

func (c *DDCClientImpl) detectAvailableDDCToolsLinux() string {
	if _, err := exec.LookPath("ddcutil"); err != nil {
		return "ddcutil"
	}

	if _, err := exec.LookPath("ddccontrol"); err != nil {
		return "ddccontrol"
	}
	return ""
}

func (c *DDCClientImpl) detectWithDdcutil() []Monitor {
	cmd := exec.Command("ddcutil", "detect")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	return c.parseDdcutilDetectOutput(string(output))
}

func (c *DDCClientImpl) parseDdcutilDetectOutput(output string) []Monitor {
	var monitors []Monitor
	lines := strings.Split(output, "\n")

	var currentMonitor *Monitor

	for _, line := range lines {
		line := strings.TrimSpace(line)

		if strings.HasPrefix(line, "Display ") {
			if currentMonitor != nil {
				monitors = append(monitors, *currentMonitor)
			}

			re := regexp.MustCompile(`Display (\d+)`)
			if matches := re.FindStringSubmatch(line); len(matches) > 1 {
				currentMonitor = &Monitor{
					ID:     matches[1],
					Inputs: make(map[string]byte),
				}
			}
		}

		if strings.Contains(line, "Mfg id:") && currentMonitor != nil {
			if mfg := extractField(line, "Mfg id:"); mfg != "" {
				currentMonitor.Name = mfg
			}
		}

		if strings.Contains(line, "Model:") && currentMonitor != nil {
			if model := extractField(line, "Model:"); model != "" {
				if currentMonitor.Name != "" {
					currentMonitor.Name += " " + model
				}
			}
		}
	}

	if currentMonitor != nil {
		monitors = append(monitors, *currentMonitor)
	}

	// Enhance each monitor with capabilities
	for i := range monitors {
		c.enhanceLinuxMonitorWithCapabilities(&monitors[i])
	}

	return monitors
}

func extractField(line, fieldName string) string {
	parts := strings.Split(line, fieldName)
	if len(parts) < 2 {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

func (c *DDCClientImpl) enhanceLinuxMonitorWithCapabilities(monitor *Monitor) {
	cmd := exec.Command("ddcutil", "--display", monitor.ID, "capabilities")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	monitor.Inputs = c.parseLinuxInputSources(string(output))

	if currentInput := c.getLinuxCurrentInput(monitor.ID); currentInput != "" {
		monitor.CurrentInput = currentInput
	}
}

func (c *DDCClientImpl) parseLinuxInputSources(capabilities string) map[string]byte {
	inputs := make(map[string]byte)

	lines := strings.Split(capabilities, "\n")
	inInputSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Feature: 60 (Input Source)") {
			inInputSection = true
			continue
		}

		if inInputSection && strings.HasPrefix(line, "Values:") {
			values := strings.Fields(strings.TrimPrefix(line, "Values: "))
			for _, hexVal := range values {
				if code, err := strconv.ParseUint(hexVal, 16, 8); err == nil {
					inputName := c.linuxInputCodeToName(byte(code))
					inputs[inputName] = byte(code)
				}
			}
			break
		}

		if inInputSection && strings.HasPrefix(line, "Feature:") {
			break
		}
	}
	return inputs

}

func (c *DDCClientImpl) linuxInputCodeToName(code byte) string {
	// Standard VCP input source codes
	switch code {
	case 0x0F:
		return "DisplayPort"
	case 0x11:
		return "HDMI-1"
	case 0x12:
		return "HDMI-2"
	case 0x13:
		return "HDMI-3"
	case 0x03:
		return "DVI-1"
	case 0x04:
		return "DVI-2"
	case 0x01:
		return "VGA"
	default:
		return fmt.Sprintf("Input-0x%02X", code)
	}
}

func (c *DDCClientImpl) getLinuxCurrentInput(monitorID string) string {
	// Get current input source value
	cmd := exec.Command("ddcutil", "--display", monitorID, "getvcp", "60")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse output like: "VCP code 0x60 (Input Source): current value = 17, max value = 255"
	re := regexp.MustCompile(`current value = (\d+)`)
	if matches := re.FindStringSubmatch(string(output)); len(matches) > 1 {
		if code, err := strconv.Atoi(matches[1]); err == nil {
			return c.linuxInputCodeToName(byte(code))
		}
	}

	return ""
}
func (c *DDCClientImpl) detectWithCoreSystem() ([]Monitor, error) {
	// First try xrandr to list monitors
	if monitors, err := c.detectWithXrandr(); err == nil && len(monitors) > 0 {
		return monitors, nil
	}
	return []Monitor{}, fmt.Errorf("no monitors detected with core system methods")
}

// Fallback method using xrandr
func (c *DDCClientImpl) detectWithXrandr() ([]Monitor, error) {
	cmd := exec.Command("xrandr", "--listmonitors")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("xrandr command failed: %w", err)
	}

	return c.parseXrandrOutput(string(output))
}

func (c *DDCClientImpl) parseXrandrOutput(output string) ([]Monitor, error) {
	var monitors []Monitor
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "Monitors:") {
			// Parse line like: " 1: +HDMI-1 2560/597x1440/336+1920+0  HDMI-1"
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				// Extract connection name (like HDMI-1, DP-1)
				connectionName := parts[len(parts)-1]

				monitor := Monitor{
					ID:           fmt.Sprintf("%d", len(monitors)+1),
					Name:         connectionName,
					Inputs:       make(map[string]byte),
					CurrentInput: "", // xrandr doesn't provide DDC info
				}

				monitors = append(monitors, monitor)
			}
		}
	}

	return monitors, nil
}

func (c *DDCClientImpl) getLinuxCapabilities(monitorID string) (*Capabilities, error) {
	// TODO: Implement using ddcutil capabilities
	// Command: ddcutil --display <id> capabilities
	return &Capabilities{}, nil
}

func (c *DDCClientImpl) setLinuxVCP(monitorID string, code byte, value uint16) error {
	// TODO: Implement using ddcutil setvcp
	// Command: ddcutil --display <id> setvcp <code> <value>
	cmdArgs := []string{"--display", monitorID, "setvcp", fmt.Sprintf("%d", code), fmt.Sprintf("%d", value)}
	cmd := exec.Command("ddcutil", cmdArgs...)
	return cmd.Run()
}

func (c *DDCClientImpl) getLinuxVCP(monitorID string, code byte) (uint16, error) {
	// TODO: Implement using ddcutil getvcp
	// Command: ddcutil --display <id> getvcp <code>
	return 0, fmt.Errorf("not implemented")
}

// ============ macOS IMPLEMENTATION ============

func (c *DDCClientImpl) detectMacOSMonitors() ([]Monitor, error) {
	// Try m1ddc first, then ddcctl
	// the ddcctl and m1ddc are not reliable in detecting monitors on macOS
	// so we are gonna go with old ways of system_profiler SPDisplaysDataType and
	baseDisplays, err := c.getSystemProfilerDisplays()
	if err == nil {
		return baseDisplays, nil
	}

	if len(baseDisplays) == 0 {
		return []Monitor{}, nil
	}

	var monitors []Monitor
	availableTool := c.detectAvailableDDCTool()
	for i, display := range baseDisplays {
		displayNum := i + 1
		enhancedDisplay := c.enhancedDisplayWithValidation(display, displayNum, availableTool)
		monitors = append(monitors, enhancedDisplay)
	}

	return monitors, nil
}

func (c *DDCClientImpl) enhancedDisplayWithValidation(baseDisplay Monitor, displayNum int, tool string) Monitor {
	enhanced := baseDisplay

	if tool == "" {
		fmt.Printf("⚠ Display %d (%s): No DDC tools installed\n", displayNum, baseDisplay.Name)
		return enhanced
	}

	validation := c.validateDDCSupport(displayNum, tool)
	switch {
	case !validation.CanReadValues:
		fmt.Printf("x Display %d (%s): %s\n", displayNum, baseDisplay.Name, validation.ValidationError)
		if validation.RecommendedAction != "" {
			fmt.Printf("  💡 Suggestion: %s\n", validation.RecommendedAction)
		}

	case !validation.CanWriteValues:
		fmt.Printf("⚠ Display %d (%s): Limited DDC support - %s\n", displayNum, baseDisplay.Name, validation.ValidationError)
		fmt.Printf("  💡 Suggestion: %s\n", validation.RecommendedAction)
		// Still try to get current values for info
		enhanced = c.addReadOnlyInfo(enhanced, displayNum, tool)
	default:
		fmt.Printf("✓ Display %d (%s): Full DDC/CI support\n", displayNum, baseDisplay.Name)
		// Full enhancement with input detection
		enhanced = c.addFullDDCInfo(enhanced, displayNum, tool)
	}
	return enhanced
}

func (c *DDCClientImpl) addReadOnlyInfo(display Monitor, displayNum int, tool string) Monitor {
	if currentInput, err := c.getCurrentInputSafe(displayNum, tool); err == nil && currentInput != 0 {
		display.CurrentInput = fmt.Sprintf("%d (read-only)", currentInput)
	}

	display.Inputs = make(map[string]byte)

	return display
}

func (c *DDCClientImpl) addFullDDCInfo(display Monitor, displayNum int, tool string) Monitor {
	if currentInput, err := c.getCurrentInputSafe(displayNum, tool); err == nil && currentInput != 0 {
		display.CurrentInput = fmt.Sprintf("%d", currentInput)
	}

	display.Inputs = c.detectAvailableInputsSafe(displayNum, tool)

	return display
}

func (c *DDCClientImpl) getCurrentInputSafe(displayNum int, tool string) (uint16, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var cmd *exec.Cmd

	switch tool {
	case "m1ddc":
		cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "get", "input")
	case "ddcctl":
		cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-i", "?")
	}

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	return c.parseVCPValue(string(output), tool, 0x60)
}

func (c *DDCClientImpl) detectAvailableInputsSafe(displayNum int, tool string) map[string]byte {
	// This is your existing detectAvailableInputs logic
	// Only call this when validation.CanWriteValues is true
	return c.detectAvailableInputs(displayNum, tool)
}

func (c *DDCClientImpl) detectAvailableInputs(displayNum int, tool string) map[string]byte {
	inputs := make(map[string]byte)
	// Test common input sources
	for inputName, code := range M1DDCInputSources {
		if c.testInputAvailable(displayNum, code, tool) {
			inputs[inputName] = byte(code)
		}
	}
	return inputs
}

func (c *DDCClientImpl) detectAvailableDDCTool() string {
	if _, err := exec.LookPath("m1ddc"); err == nil {
		return "m1ddc"
	}
	if _, err := exec.LookPath("ddcctl"); err == nil {
		return "ddcctl"
	}

	if _, err := exec.LookPath("ddcutil"); err != nil {
		return "ddcutil"
	}

	if _, err := exec.LookPath("ddccontrol"); err != nil {
		return "ddccontrol"
	}
	return ""
}

func (c *DDCClientImpl) validateDDCSupport(displayNum int, tool string) *DDCValidationResult {
	result := &DDCValidationResult{
		ToolAvailable: tool != "",
	}

	if !result.ToolAvailable {
		result.ValidationError = fmt.Errorf("no DDC tool available")
		result.RecommendedAction = "Install 'm1ddc' or 'ddcctl' to enable DDC functionality."
		return result
	}

	currentBrightness, err := c.testReadBrightness(displayNum, tool)
	if err != nil {
		result.ValidationError = fmt.Errorf("Cannot read brightness: %v", err)
		result.RecommendedAction = "Monitor may not support DDC/CI or connection issue"
		return result
	}
	result.CanReadValues = true

	// Test 2: Can we write brightness and does it actually change?
	if !c.testWriteBrightness(displayNum, tool, currentBrightness) {
		result.CanWriteValues = false
		result.ValidationError = fmt.Errorf("DDC commands execute but have no effect")
		result.RecommendedAction = "VGA/older connections often don't support DDC control. Try HDMI/DisplayPort"
		return result
	}
	result.CanWriteValues = true

	return result
}

func (c *DDCClientImpl) testReadBrightness(displayNum int, tool string) (uint16, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	var cmd *exec.Cmd

	switch tool {
	case "m1ddc":
		cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "get", "luminance")
	case "ddcctl":
		cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-b", "?")
	}

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	return c.parseVCPValue(string(output), tool, 0x10)
}

func (c *DDCClientImpl) testWriteBrightness(displayNum int, tool string, originalBrightness uint16) bool {
	testValue := originalBrightness + 10

	if testValue > 100 {
		if originalBrightness >= 10 {
			testValue = originalBrightness - 10
		} else {
			testValue = 50
		}
	}

	if err := c.setBrightnessValue(displayNum, tool, testValue); err != nil {
		return false
	}

	time.Sleep(500 * time.Millisecond)

	newBrightness, err := c.testReadBrightness(displayNum, tool)
	if err != nil {
		return false
	}

	c.setBrightnessValue(displayNum, tool, originalBrightness)

	return newBrightness == testValue
}

func (c *DDCClientImpl) setBrightnessValue(displayNum int, tool string, value uint16) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var cmd *exec.Cmd

	switch tool {
	case "m1ddc":
		cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "set", "luminance", strconv.Itoa(int(value)))
	case "ddcctl":
		cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-b", strconv.Itoa(int(value)))
	}

	return cmd.Run()
}

func (c *DDCClientImpl) testInputAvailable(displayNum int, inputCode int, tool string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch tool {
	case "ddcctl":
		// Try to set this input
		cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-i", strconv.Itoa(inputCode))
	case "m1ddc":
		// Try to set this input
		cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "set", "input", strconv.Itoa(inputCode))
	}

	// Suppress output to avoid noise during testing
	cmd.Stdout = nil
	cmd.Stderr = nil

	err := cmd.Run()
	return err == nil
}

//	{
//	  "SPDisplaysDataType" : [
//	    {
//	      "_name" : "kHW_AppleM1Item",
//	      "spdisplays_mtlgpufamilysupport" : "spdisplays_metal3",
//	      "spdisplays_ndrvs" : [
//	        {
//	          "_name" : "Color LCD",
//	          "_spdisplays_display-product-id" : "a045",
//	          "_spdisplays_display-serial-number" : "fd626d62",
//	          "_spdisplays_display-vendor-id" : "610",
//	          "_spdisplays_display-week" : "0",
//	          "_spdisplays_display-year" : "0",
//	          "_spdisplays_displayID" : "1",
//	          "_spdisplays_pixels" : "3360 x 2100",
//	          "_spdisplays_resolution" : "1680 x 1050 @ 60.00Hz",
//	          "spdisplays_ambient_brightness" : "spdisplays_yes",
//	          "spdisplays_connection_type" : "spdisplays_internal",
//	          "spdisplays_display_type" : "spdisplays_built-in_retinaLCD",
//	          "spdisplays_main" : "spdisplays_yes",
//	          "spdisplays_mirror" : "spdisplays_off",
//	          "spdisplays_online" : "spdisplays_yes",
//	          "spdisplays_pixelresolution" : "spdisplays_2560x1600Retina"
//	        }
//	      ],
//	      "spdisplays_vendor" : "sppci_vendor_Apple",
//	      "sppci_bus" : "spdisplays_builtin",
//	      "sppci_cores" : "7",
//	      "sppci_device_type" : "spdisplays_gpu",
//	      "sppci_model" : "Apple M1"
//	    }
//	  ]
//	}
//
// Example for struct to parse system_profiler SPDisplaysDataType JSON output
type SystemProfilerOutput struct {
	SPDisplaysDataType []struct {
		Name  string `json:"_name"`
		Ndrvs []struct {
			Name                string `json:"_name"`
			DisplayProductID    string `json:"_spdisplays_display-product-id"`
			DisplaySerialNumber string `json:"_spdisplays_display-serial-number"`
			DisplayVendorID     string `json:"_spdisplays_display-vendor-id"`
			DisplayWeek         string `json:"_spdisplays_display-week"`
			DisplayYear         string `json:"_spdisplays_display-year"`
			DisplayID           string `json:"_spdisplays_displayID"`
			Pixels              string `json:"_spdisplays_pixels"`
			Resolution          string `json:"_spdisplays_resolution"`
			AmbientBrightness   string `json:"spdisplays_ambient_brightness"`
			ConnectionType      string `json:"spdisplays_connection_type"`
			DisplayType         string `json:"spdisplays_display_type"`
			Main                string `json:"spdisplays_main"`
			Mirror              string `json:"spdisplays_mirror"`
			Online              string `json:"spdisplays_online"`
			Pixelresolution     string `json:"spdisplays_pixelresolution"`
		} `json:"spdisplays_ndrvs"`
	} `json:"SPDisplaysDataType"`
}

func (c *DDCClientImpl) getSystemProfilerDisplays() ([]Monitor, error) {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType", "-json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("system_profiler command failed: %v", err)
	}

	var spOutput SystemProfilerOutput
	err = json.Unmarshal(output, &spOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to parse system_profiler output: %v", err)
	}
	var monitors []Monitor
	for _, display := range spOutput.SPDisplaysDataType {
		for _, ndrv := range display.Ndrvs {
			if ndrv.ConnectionType == "spdisplays_internal" {
				continue
			} else {
				monitor := Monitor{
					ID:   ndrv.DisplayID,
					Name: ndrv.Name,
					// Inputs and CurrentInput are not available via system_profiler
					Inputs:       map[string]byte{},
					CurrentInput: "",
				}
				if ndrv.Name != "" && ndrv.Name != "(null)" {
					monitor.Name = ndrv.Name
				} else {
					monitor.Name = c.getDisplayName(ndrv)
				}
				monitors = append(monitors, monitor)
			}
		}
	}
	if len(monitors) == 0 {
		return nil, fmt.Errorf("no external monitors found in system_profiler output")
	}
	return monitors, nil

}
func (c *DDCClientImpl) getVendorName(vendorID string) string {
	// Convert hex vendor ID to known manufacturer names
	knownVendors := map[string]string{
		"610":  "Apple",
		"5e3":  "ASUS",
		"10ac": "Dell",
		"1e6d": "LG",
		"4c2d": "Samsung",
		// Add more as needed
	}

	if vendor, exists := knownVendors[vendorID]; exists {
		return vendor
	}

	return ""
}
func (c *DDCClientImpl) getDisplayName(ndrv struct {
	Name                string `json:"_name"`
	DisplayProductID    string `json:"_spdisplays_display-product-id"`
	DisplaySerialNumber string `json:"_spdisplays_display-serial-number"`
	DisplayVendorID     string `json:"_spdisplays_display-vendor-id"`
	DisplayWeek         string `json:"_spdisplays_display-week"`
	DisplayYear         string `json:"_spdisplays_display-year"`
	DisplayID           string `json:"_spdisplays_displayID"`
	Pixels              string `json:"_spdisplays_pixels"`
	Resolution          string `json:"_spdisplays_resolution"`
	AmbientBrightness   string `json:"spdisplays_ambient_brightness"`
	ConnectionType      string `json:"spdisplays_connection_type"`
	DisplayType         string `json:"spdisplays_display_type"`
	Main                string `json:"spdisplays_main"`
	Mirror              string `json:"spdisplays_mirror"`
	Online              string `json:"spdisplays_online"`
	Pixelresolution     string `json:"spdisplays_pixelresolution"`
}) string {
	// If system_profiler provides a name, use it
	if ndrv.Name != "" && ndrv.Name != "(null)" {
		return ndrv.Name
	}

	// Build name from vendor/product IDs
	vendorName := c.getVendorName(ndrv.DisplayVendorID)
	if vendorName != "" {
		return fmt.Sprintf("%s Display", vendorName)
	}

	// Fallback to generic name
	return fmt.Sprintf("External Display %s", ndrv.DisplayID)
}

func (c *DDCClientImpl) getMacOSCapabilities(monitorID string) (*Capabilities, error) {
	// TODO: Implement capabilities detection for macOS
	return &Capabilities{}, nil
}

// SetVCP for macOS with correct command syntax
func (c *DDCClientImpl) setMacOSVCP(monitorID string, code byte, value uint16) error {
	displayNum, err := strconv.Atoi(monitorID)
	if err != nil {
		return fmt.Errorf("invalid monitor ID: %s", monitorID)
	}

	tool := c.detectAvailableDDCTool()
	if tool == "" {
		return fmt.Errorf("no DDC tools available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch tool {
	case "ddcctl":
		switch code {
		case 0x10: // Brightness
			cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-b", strconv.Itoa(int(value)))
		case 0x12: // Contrast
			cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-c", strconv.Itoa(int(value)))
		case 0x60: // Input Source
			cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-i", strconv.Itoa(int(value)))
		case 0x62: // Volume
			cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-v", strconv.Itoa(int(value)))
		default:
			return fmt.Errorf("unsupported VCP code for ddcctl: 0x%02X", code)
		}
	case "m1ddc":
		switch code {
		case 0x10: // Brightness (luminance in m1ddc)
			cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "set", "luminance", strconv.Itoa(int(value)))
		case 0x12: // Contrast
			cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "set", "contrast", strconv.Itoa(int(value)))
		case 0x60: // Input Source
			cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "set", "input", strconv.Itoa(int(value)))
		case 0x62: // Volume
			cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "set", "volume", strconv.Itoa(int(value)))
		default:
			return fmt.Errorf("unsupported VCP code for m1ddc: 0x%02X", code)
		}
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set VCP 0x%02X to %d: %w", code, value, err)
	}

	return nil
}

// GetVCP for macOS with correct command syntax
func (c *DDCClientImpl) getMacOSVCP(monitorID string, code byte) (uint16, error) {
	displayNum, err := strconv.Atoi(monitorID)
	if err != nil {
		return 0, fmt.Errorf("invalid monitor ID: %s", monitorID)
	}

	tool := c.detectAvailableDDCTool()
	if tool == "" {
		return 0, fmt.Errorf("no DDC tools available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch tool {
	case "ddcctl":
		switch code {
		case 0x10: // Brightness
			cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-b", "?")
		case 0x12: // Contrast
			cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-c", "?")
		case 0x60: // Input Source
			cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-i", "?")
		case 0x62: // Volume
			cmd = exec.CommandContext(ctx, "ddcctl", "-d", strconv.Itoa(displayNum), "-v", "?")
		default:
			return 0, fmt.Errorf("unsupported VCP code for ddcctl: 0x%02X", code)
		}
	case "m1ddc":
		switch code {
		case 0x10: // Brightness
			cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "get", "luminance")
		case 0x12: // Contrast
			cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "get", "contrast")
		case 0x60: // Input Source
			cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "get", "input")
		case 0x62: // Volume
			cmd = exec.CommandContext(ctx, "m1ddc", "display", strconv.Itoa(displayNum), "get", "volume")
		}
	}

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get VCP 0x%02X: %w", code, err)
	}

	// Parse the output to extract the value
	value, err := c.parseVCPValue(string(output), tool, code)
	if err != nil {
		return 0, fmt.Errorf("failed to parse VCP value from '%s': %w", strings.TrimSpace(string(output)), err)
	}

	return value, nil
}
func (c *DDCClientImpl) parseVCPValue(output, tool string, code byte) (uint16, error) {
	// Clean up the output
	output = strings.TrimSpace(output)

	switch tool {
	case "ddcctl":
		// ddcctl output examples:
		// "control #16 = 75" (brightness)
		// "Display 2: brightness = 75"
		patterns := []string{
			`control\s+#\d+\s+=\s+(\d+)`,
			`brightness\s*=\s*(\d+)`,
			`contrast\s*=\s*(\d+)`,
			`volume\s*=\s*(\d+)`,
			`input\s*=\s*(\d+)`,
			`(\d+)`, // Fallback: just find any number
		}

		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(output); len(matches) > 1 {
				value, err := strconv.Atoi(matches[1])
				if err == nil {
					return uint16(value), nil
				}
			}
		}

	case "m1ddc":
		// m1ddc output examples:
		// "75" (simple number)
		// "Current luminance: 75"
		patterns := []string{
			`luminance:\s*(\d+)`,
			`contrast:\s*(\d+)`,
			`volume:\s*(\d+)`,
			`input:\s*(\d+)`,
			`^\s*(\d+)\s*$`, // Just a number by itself
		}

		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(output); len(matches) > 1 {
				value, err := strconv.Atoi(matches[1])
				if err == nil {
					return uint16(value), nil
				}
			}
		}
	}

	return 0, fmt.Errorf("could not parse value from output: '%s'", output)
}

// ============ WINDOWS IMPLEMENTATION ============

func (c *DDCClientImpl) detectWindowsMonitors() ([]Monitor, error) {
	// TODO: Implement Windows monitor detection
	return []Monitor{}, fmt.Errorf("Windows DDC not implemented yet")
}

func (c *DDCClientImpl) getWindowsCapabilities(monitorID string) (*Capabilities, error) {
	return &Capabilities{}, fmt.Errorf("Windows capabilities not implemented yet")
}

func (c *DDCClientImpl) setWindowsVCP(monitorID string, code byte, value uint16) error {
	return fmt.Errorf("Windows VCP setting not implemented yet")
}

func (c *DDCClientImpl) getWindowsVCP(monitorID string, code byte) (uint16, error) {
	return 0, fmt.Errorf("Windows VCP getting not implemented yet")
}
