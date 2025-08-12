package uixt

import (
	"context"
	"fmt"
	"os"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"

	"github.com/httprunner/httprunner/v5/pkg/gadb"
	"github.com/httprunner/httprunner/v5/uixt/option"
)

// ToolListAvailableDevices implements the list_available_devices tool call.
type ToolListAvailableDevices struct {
	// Return data fields - these define the structure of data returned by this tool
	AndroidDevices []string `json:"androidDevices" desc:"List of Android device serial numbers"`
	IosDevices     []string `json:"iosDevices" desc:"List of iOS device UDIDs"`
	TotalCount     int      `json:"totalCount" desc:"Total number of available devices"`
	AndroidCount   int      `json:"androidCount" desc:"Number of Android devices"`
	IosCount       int      `json:"iosCount" desc:"Number of iOS devices"`
}

func (t *ToolListAvailableDevices) Name() option.ActionName {
	return option.ACTION_ListAvailableDevices
}

func (t *ToolListAvailableDevices) Description() string {
	return "List all available devices including Android devices and iOS devices. If there are multiple devices returned, you need to let the user select one of them."
}

func (t *ToolListAvailableDevices) Options() []mcp.ToolOption {
	return []mcp.ToolOption{}
}

func (t *ToolListAvailableDevices) Implement() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		deviceList := make(map[string][]string)
		if client, err := gadb.NewClient(); err == nil {
			if androidDevices, err := client.DeviceList(); err == nil {
				serialList := make([]string, 0, len(androidDevices))
				for _, device := range androidDevices {
					serialList = append(serialList, device.Serial())
				}
				deviceList["androidDevices"] = serialList
			}
		}
		if iosDevices, err := ios.ListDevices(); err == nil {
			serialList := make([]string, 0, len(iosDevices.DeviceList))
			for _, dev := range iosDevices.DeviceList {
				device, err := NewIOSDevice(
					option.WithUDID(dev.Properties.SerialNumber))
				if err != nil {
					continue
				}
				properties := device.Properties
				err = ios.Pair(dev)
				if err != nil {
					log.Error().Err(err).Msg("failed to pair device")
					continue
				}
				serialList = append(serialList, properties.SerialNumber)
			}
			deviceList["iosDevices"] = serialList
		}

		// Create structured response
		totalDevices := len(deviceList["androidDevices"]) + len(deviceList["iosDevices"])
		message := fmt.Sprintf("Found %d available devices (%d Android, %d iOS)",
			totalDevices, len(deviceList["androidDevices"]), len(deviceList["iosDevices"]))
		returnData := ToolListAvailableDevices{
			AndroidDevices: deviceList["androidDevices"],
			IosDevices:     deviceList["iosDevices"],
			TotalCount:     totalDevices,
			AndroidCount:   len(deviceList["androidDevices"]),
			IosCount:       len(deviceList["iosDevices"]),
		}

		return NewMCPSuccessResponse(message, &returnData), nil
	}
}

func (t *ToolListAvailableDevices) ConvertActionToCallToolRequest(action option.MobileAction) (mcp.CallToolRequest, error) {
	return BuildMCPCallToolRequest(t.Name(), map[string]any{}, action), nil
}

// ToolSelectDevice implements the select_device tool call.
type ToolSelectDevice struct {
	// Return data fields - these define the structure of data returned by this tool
	DeviceUUID string `json:"deviceUUID" desc:"UUID of the selected device"`
}

func (t *ToolSelectDevice) Name() option.ActionName {
	return option.ACTION_SelectDevice
}

func (t *ToolSelectDevice) Description() string {
	return "Select a device to use from the list of available devices. Use the list_available_devices tool first to get a list of available devices."
}

func (t *ToolSelectDevice) Options() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("platform", mcp.Enum("android", "ios"), mcp.Description("The platform type of device to select")),
		mcp.WithString("serial", mcp.Description("The device serial number or UDID to select")),
	}
}

func (t *ToolSelectDevice) Implement() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		driverExt, err := setupXTDriver(ctx, request.GetArguments())
		if err != nil {
			return nil, err
		}

		uuid := driverExt.IDriver.GetDevice().UUID()
		message := fmt.Sprintf("Selected device: %s", uuid)
		returnData := ToolSelectDevice{DeviceUUID: uuid}

		return NewMCPSuccessResponse(message, &returnData), nil
	}
}

func (t *ToolSelectDevice) ConvertActionToCallToolRequest(action option.MobileAction) (mcp.CallToolRequest, error) {
	return BuildMCPCallToolRequest(t.Name(), map[string]any{}, action), nil
}

// ToolScreenRecord implements the screenrecord tool call.
type ToolScreenRecord struct {
	// Return data fields - these define the structure of data returned by this tool
	VideoPath string  `json:"videoPath" desc:"Path to the recorded video file"`
	Duration  float64 `json:"duration" desc:"Duration of the recording in seconds"`
	Method    string  `json:"method" desc:"Recording method used (adb or scrcpy)"`
}

func (t *ToolScreenRecord) Name() option.ActionName {
	return option.ACTION_ScreenRecord
}

func (t *ToolScreenRecord) Description() string {
	return "Record the screen of the mobile device. Supports both ADB screenrecord and scrcpy recording methods. ADB recording is limited to 180 seconds, while scrcpy supports longer recordings and audio capture on Android 11+."
}

func (t *ToolScreenRecord) Options() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("platform", mcp.Enum("android", "ios"), mcp.Description("The platform type of device to record")),
		mcp.WithString("serial", mcp.Description("The device serial number or UDID")),
		mcp.WithNumber("duration", mcp.Description("Recording duration in seconds. If not specified, recording will continue until manually stopped. ADB recording is limited to 180 seconds.")),
		mcp.WithString("screenRecordPath", mcp.Description("Custom path for the output video file. If not specified, a timestamped filename will be generated.")),
		mcp.WithBoolean("screenRecordWithAudio", mcp.Description("Enable audio recording (requires scrcpy and Android 11+). Default: false")),
		mcp.WithBoolean("screenRecordWithScrcpy", mcp.Description("Force use of scrcpy for recording instead of ADB. Default: false (auto-detect based on audio requirement)")),
	}
}

func (t *ToolScreenRecord) Implement() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := request.GetArguments()
		driverExt, err := setupXTDriver(ctx, arguments)
		if err != nil {
			return nil, err
		}

		// Parse options from arguments
		var opts []option.ActionOption

		if duration, ok := arguments["duration"].(float64); ok && duration > 0 {
			opts = append(opts, option.WithDuration(duration))
		}

		if path, ok := arguments["screenRecordPath"].(string); ok && path != "" {
			opts = append(opts, option.WithScreenRecordPath(path))
		}

		if audio, ok := arguments["screenRecordWithAudio"].(bool); ok && audio {
			opts = append(opts, option.WithScreenRecordAudio(true))
		}

		if scrcpy, ok := arguments["screenRecordWithScrcpy"].(bool); ok && scrcpy {
			opts = append(opts, option.WithScreenRecordScrcpy(true))
		}

		// Add context to options for proper cancellation handling
		opts = append(opts, option.WithContext(ctx))

		// Start screen recording
		videoPath, err := driverExt.IDriver.ScreenRecord(opts...)
		if err != nil {
			log.Error().Err(err).Msg("ScreenRecord failed")
			return NewMCPErrorResponse("Failed to record screen: " + err.Error()), nil
		}

		// Determine recording method and duration
		options := option.NewActionOptions(opts...)
		method := "adb"
		duration := options.Duration
		if options.ScreenRecordDuration > 0 {
			duration = options.ScreenRecordDuration
		}

		if options.ScreenRecordWithScrcpy || options.ScreenRecordWithAudio {
			method = "scrcpy"
		}

		message := fmt.Sprintf("Screen recording completed successfully. Video saved to: %s", videoPath)
		returnData := ToolScreenRecord{
			VideoPath: videoPath,
			Duration:  duration,
			Method:    method,
		}

		return NewMCPSuccessResponse(message, &returnData), nil
	}
}

func (t *ToolScreenRecord) ConvertActionToCallToolRequest(action option.MobileAction) (mcp.CallToolRequest, error) {
	return BuildMCPCallToolRequest(t.Name(), map[string]any{}, action), nil
}

// ToolPushImage implements the push_image tool call.
type ToolPushImage struct {
	// Return data fields - these define the structure of data returned by this tool
	ImagePath string `json:"imagePath" desc:"Path of the image that was pushed"`
	ImageUrl  string `json:"imageUrl,omitempty" desc:"URL of the image that was downloaded and pushed (if applicable)"`
	Cleared   bool   `json:"cleared,omitempty" desc:"Whether images were cleared before pushing (if applicable)"`
}

func (t *ToolPushImage) Name() option.ActionName {
	return option.ACTION_PushImage
}

func (t *ToolPushImage) Description() string {
	return "Push an image to the device's gallery. For Android, the image will be pushed to the DCIM/Camera directory. For iOS, the image will be added to the device's photo album."
}

func (t *ToolPushImage) Options() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("platform", mcp.Enum("android", "ios"), mcp.Description("The platform type of device to push image to")),
		mcp.WithString("serial", mcp.Description("The device serial number or UDID")),
		mcp.WithString("imagePath", mcp.Description("Path to the local image file to push to the device")),
		mcp.WithString("imageUrl", mcp.Description("URL of the image to download and push to the device")),
		mcp.WithBoolean("cleanup", mcp.Description("Whether to delete the downloaded file after pushing it to the device")),
		mcp.WithBoolean("clearBefore", mcp.Description("Whether to clear images before pushing (if applicable)")),
	}
}

func (t *ToolPushImage) Implement() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		driverExt, err := setupXTDriver(ctx, request.GetArguments())
		if err != nil {
			return nil, err
		}

		// Get image path or URL
		imagePath, hasPath := request.GetArguments()["imagePath"].(string)
		imageUrl, hasUrl := request.GetArguments()["imageUrl"].(string)
		cleanup, _ := request.GetArguments()["cleanup"].(bool)
		clearBefore, _ := request.GetArguments()["clearBefore"].(bool)

		// Check if we have either path or URL
		if (!hasPath || imagePath == "") && (!hasUrl || imageUrl == "") {
			return nil, fmt.Errorf("either imagePath or imageUrl is required")
		}

		// If we have a URL, download it
		downloadedFile := false
		if hasUrl && imageUrl != "" {
			log.Info().Str("imageUrl", imageUrl).Msg("Downloading image from URL")
			downloadedPath, err := DownloadFileByUrl(imageUrl)
			if err != nil {
				return nil, fmt.Errorf("failed to download image from URL: %v", err)
			}

			// Detect image type and rename with proper extension
			renamedPath, err := DetectAndRenameImageFile(downloadedPath)
			if err != nil {
				log.Warn().Err(err).Str("path", downloadedPath).Msg("Failed to detect image type or rename file, using original file")
				imagePath = downloadedPath
			} else {
				imagePath = renamedPath
			}
			downloadedFile = true
		}

		// Clear images before pushing if requested
		cleared := false
		if clearBefore {
			log.Info().Msg("Clearing images before pushing new image")
			err := driverExt.IDriver.ClearImages()
			if err != nil {
				log.Warn().Err(err).Msg("Failed to clear images before pushing, continuing anyway")
			} else {
				cleared = true
			}
		}

		// Push the image to the device
		err = driverExt.IDriver.PushImage(imagePath)
		if err != nil {
			// If we downloaded the file and failed to push it, clean up
			if downloadedFile && cleanup {
				_ = os.Remove(imagePath)
			}
			return nil, err
		}

		// Clean up downloaded file if requested
		if downloadedFile && cleanup {
			log.Info().Str("imagePath", imagePath).Msg("Cleaning up downloaded image")
			_ = os.Remove(imagePath)
		}

		message := fmt.Sprintf("Successfully pushed image to device")
		returnData := ToolPushImage{
			ImagePath: imagePath,
			Cleared:   cleared,
		}

		// Include URL in response if it was used
		if hasUrl && imageUrl != "" {
			returnData.ImageUrl = imageUrl
			message = fmt.Sprintf("Successfully downloaded and pushed image from %s to device", imageUrl)
		}

		// Add cleared info to message if applicable
		if cleared {
			message = fmt.Sprintf("%s (images cleared before pushing)", message)
		}

		return NewMCPSuccessResponse(message, &returnData), nil
	}
}

func (t *ToolPushImage) ConvertActionToCallToolRequest(action option.MobileAction) (mcp.CallToolRequest, error) {
	arguments := map[string]any{}

	// Handle string param as imageUrl
	if imageUrl, ok := action.Params.(string); ok && imageUrl != "" {
		arguments["imageUrl"] = imageUrl
	}

	// Handle map params with imageUrl or imagePath
	if params, ok := action.Params.(map[string]interface{}); ok {
		if imageUrl, ok := params["imageUrl"].(string); ok && imageUrl != "" {
			arguments["imageUrl"] = imageUrl
		}
		if imagePath, ok := params["imagePath"].(string); ok && imagePath != "" {
			arguments["imagePath"] = imagePath
		}
		if cleanup, ok := params["cleanup"].(bool); ok {
			arguments["cleanup"] = cleanup
		}
		if clearBefore, ok := params["clearBefore"].(bool); ok {
			arguments["clearBefore"] = clearBefore
		}
	}

	// Handle custom options
	if imageUrl, ok := action.ActionOptions.Custom["imageUrl"].(string); ok && imageUrl != "" {
		arguments["imageUrl"] = imageUrl
	}
	if imagePath, ok := action.ActionOptions.Custom["imagePath"].(string); ok && imagePath != "" {
		arguments["imagePath"] = imagePath
	}
	if cleanup, ok := action.ActionOptions.Custom["cleanup"].(bool); ok {
		arguments["cleanup"] = cleanup
	}
	if clearBefore, ok := action.ActionOptions.Custom["clearBefore"].(bool); ok {
		arguments["clearBefore"] = clearBefore
	}

	return BuildMCPCallToolRequest(t.Name(), arguments, action), nil
}

// ToolClearImage implements the clear_image tool call.
type ToolClearImage struct {
	// Return data fields - these define the structure of data returned by this tool
	Success bool `json:"success" desc:"Whether the operation was successful"`
}

func (t *ToolClearImage) Name() option.ActionName {
	return option.ACTION_ClearImage
}

func (t *ToolClearImage) Description() string {
	return "Clear images from the device's gallery. For Android, this will remove all images from the DCIM/Camera directory. For iOS, this will clear the images added through the push_image tool."
}

func (t *ToolClearImage) Options() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("platform", mcp.Enum("android", "ios"), mcp.Description("The platform type of device to clear images from")),
		mcp.WithString("serial", mcp.Description("The device serial number or UDID")),
	}
}

func (t *ToolClearImage) Implement() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		driverExt, err := setupXTDriver(ctx, request.GetArguments())
		if err != nil {
			return nil, err
		}

		err = driverExt.IDriver.ClearImages()
		if err != nil {
			return nil, err
		}

		message := "Successfully cleared images from device"
		returnData := ToolClearImage{Success: true}

		return NewMCPSuccessResponse(message, &returnData), nil
	}
}

func (t *ToolClearImage) ConvertActionToCallToolRequest(action option.MobileAction) (mcp.CallToolRequest, error) {
	return BuildMCPCallToolRequest(t.Name(), map[string]any{}, action), nil
}
