#import "ddc_macos.h"
#import <CoreGraphics/CoreGraphics.h>
#import <Foundation/Foundation.h>
#import <IOKit/graphics/IOGraphicsLib.h>
#import <IOKit/i2c/IOI2CInterface.h>

#define kMaxDisplays 16
#define kDDCMinReplyDelay 30000000 // 30ms in nanoseconds
#define kDDCMaxIterations 128

// DDC/CI command structures
struct DDCWriteCommand {
  UInt8 control_id;
  UInt8 new_value;
};

struct DDCReadCommand {
  UInt8 control_id;
  UInt8 max_value;
  UInt8 current_value;
};
// Helper: Get IO service port for display (replacement for deprecated
// CGDisplayIOServicePort) This function finds the IOService that corresponds to
// a given CGDirectDisplayID by matching vendor ID, product ID, and serial
// number
static io_service_t
GetIOServicePortFromCGDisplayID(CGDirectDisplayID displayID) {
  io_iterator_t iter;
  io_service_t serv, servicePort = 0;

  // Create a matching dictionary for IODisplayConnect services
  // IODisplayConnect represents the connection between GPU and display
  CFMutableDictionaryRef matching = IOServiceMatching("IODisplayConnect");

  // Get all services that match "IODisplayConnect"
  // kIOMasterPortDefault: uses the default master port for IOKit communication
  // matching: the dictionary describing what we're looking for
  // &iter: outputs an iterator to loop through matching services
  kern_return_t err =
      IOServiceGetMatchingServices(kIOMasterPortDefault, matching, &iter);
  if (err) {
    return 0;
  }

  // Loop through all IODisplayConnect services
  while ((serv = IOIteratorNext(iter)) != 0) {
    CFDictionaryRef displayInfo;
    CFNumberRef vendorIDRef;
    CFNumberRef productIDRef;
    CFNumberRef serialNumberRef;

    // Get the display information dictionary for this service
    // kIODisplayOnlyPreferredName: only get the preferred (localized) name
    displayInfo =
        IODisplayCreateInfoDictionary(serv, kIODisplayOnlyPreferredName);

    Boolean success;
    // Try to get vendor ID from the info dictionary
    // CFSTR(kDisplayVendorID): constant string key for vendor ID
    // &vendorIDRef: outputs the CFNumber containing vendor ID
    success = CFDictionaryGetValueIfPresent(
        displayInfo, CFSTR(kDisplayVendorID), (const void **)&vendorIDRef);
    // Try to get product ID
    success &= CFDictionaryGetValueIfPresent(
        displayInfo, CFSTR(kDisplayProductID), (const void **)&productIDRef);

    if (!success) {
      CFRelease(displayInfo);
      continue;
    }

    SInt32 vendorID;
    // Extract the actual integer value from CFNumber
    // kCFNumberSInt32Type: tells it we want a 32-bit signed integer
    CFNumberGetValue(vendorIDRef, kCFNumberSInt32Type, &vendorID);
    SInt32 productID;
    CFNumberGetValue(productIDRef, kCFNumberSInt32Type, &productID);

    // Serial number is optional - some displays don't have it
    // Initialize to 0 which will match CGDisplaySerialNumber if not present
    SInt32 serialNumber = 0;
    if (CFDictionaryGetValueIfPresent(displayInfo, CFSTR(kDisplaySerialNumber),
                                      (const void **)&serialNumberRef)) {
      CFNumberGetValue(serialNumberRef, kCFNumberSInt32Type, &serialNumber);
    }

    // Check if this service matches our target display
    // Compare: vendor ID, product ID, and serial number
    // This ensures we get the right display even with multiple identical
    // monitors
    if (CGDisplayVendorNumber(displayID) != vendorID ||
        CGDisplayModelNumber(displayID) != productID ||
        CGDisplaySerialNumber(displayID) != serialNumber) {
      CFRelease(displayInfo);
      continue;
    }

    // Found it! Save the service port and exit loop
    servicePort = serv;
    CFRelease(displayInfo);
    break;
  }

  IOObjectRelease(iter);
  return servicePort;
}
// Helper: Get IO service port for display
// Helper: Get IO service for I2C communication with display
// This gets the actual I2C interface we need for DDC/CI commands
static io_service_t GetIOServicePort(CGDirectDisplayID displayID) {
  io_service_t displayService = GetIOServicePortFromCGDisplayID(displayID);
  if (!displayService) {
    return 0;
  }

  // Now we need to find the I2C interface for this display
  io_iterator_t iter;
  io_service_t service = 0;

  // Look for IOFramebufferI2CInterface services that are children of our
  // display
  CFMutableDictionaryRef matching =
      IOServiceMatching("IOFramebufferI2CInterface");

  // Get matching services
  kern_return_t err =
      IOServiceGetMatchingServices(kIOMasterPortDefault, matching, &iter);
  if (err != kIOReturnSuccess) {
    IOObjectRelease(displayService);
    return 0;
  }

  // Find the I2C interface that belongs to our display
  io_service_t candidate;
  while ((candidate = IOIteratorNext(iter)) != 0) {
    // Check if this I2C interface belongs to our display
    // We do this by checking the parent IOService
    io_service_t parent;
    if (IORegistryEntryGetParentEntry(candidate, kIOServicePlane, &parent) ==
        kIOReturnSuccess) {
      // Get the display's vendor/product to compare
      CFNumberRef vendorRef = IORegistryEntryCreateCFProperty(
          parent, CFSTR(kDisplayVendorID), kCFAllocatorDefault, kNilOptions);
      CFNumberRef productRef = IORegistryEntryCreateCFProperty(
          parent, CFSTR(kDisplayProductID), kCFAllocatorDefault, kNilOptions);

      if (vendorRef && productRef) {
        SInt32 vendor, product;
        CFNumberGetValue(vendorRef, kCFNumberSInt32Type, &vendor);
        CFNumberGetValue(productRef, kCFNumberSInt32Type, &product);

        // Does it match our target display?
        if (vendor == CGDisplayVendorNumber(displayID) &&
            product == CGDisplayModelNumber(displayID)) {
          service = candidate;
          if (vendorRef)
            CFRelease(vendorRef);
          if (productRef)
            CFRelease(productRef);
          IOObjectRelease(parent);
          break;
        }
      }

      if (vendorRef)
        CFRelease(vendorRef);
      if (productRef)
        CFRelease(productRef);
      IOObjectRelease(parent);
    }
    IOObjectRelease(candidate);
  }

  IOObjectRelease(iter);
  IOObjectRelease(displayService);
  return service;
}

char *GetMonitorsJSON() {
  @autoreleasepool {
    // What: Creates a temporary memory pool for Objective-C objects
    // Why: Objects created inside are automatically freed when the block ends
    // Similar to: Go's defer but for a whole scope
    CGDirectDisplayID displays[kMaxDisplays];
    uint32_t displayCount = 0;

    if (CGGetOnlineDisplayList(kMaxDisplays, displays, &displayCount) !=
        kCGErrorSuccess) {
      return strdup("[]");
    }

    NSMutableArray *monitors = [NSMutableArray array];
    for (uint32_t i = 0; i < displayCount; i++) {
      CGDirectDisplayID displayID = displays[i];

      // skip built-in displays
      if (CGDisplayIsBuiltin(displayID)) {
        continue;
      }

      NSMutableDictionary *monitor = [NSMutableDictionary dictionary];
      monitor[@"id"] = @(displayID);
      // Get display info from IOKit
      io_service_t service = GetIOServicePortFromCGDisplayID(displayID);
      if (service) {
        CFDictionaryRef info =
            IODisplayCreateInfoDictionary(service, kIODisplayOnlyPreferredName);
        if (info) {
          NSDictionary *names = (__bridge NSDictionary *)CFDictionaryGetValue(
              info, CFSTR(kDisplayProductName));
          if (names && [names count] > 0) {
            NSString *name = [names objectForKey:[[names allKeys] firstObject]];
            if (name) {
              monitor[@"name"] = name;
            }
          }
          CFRelease(info); // Add this - we own this reference
        }
        IOObjectRelease(service); // Add this - clean up the service
      }

      // Fallback name
      if (!monitor[@"name"]) {
        monitor[@"name"] = [NSString stringWithFormat:@"Display %u", displayID];
      }

      monitor[@"vendor_id"] = @(CGDisplayVendorNumber(displayID));
      monitor[@"model_id"] = @(CGDisplayModelNumber(displayID));
      monitor[@"serial"] = @(CGDisplaySerialNumber(displayID));

      [monitors addObject:monitor];
    }

    NSError *error = nil;
    NSData *jsonData =
        [NSJSONSerialization dataWithJSONObject:monitors
                                        options:NSJSONWritingPrettyPrinted
                                          error:&error];
    if (error) {
      return strdup("[]");
    }
    NSString *jsonString = [[NSString alloc] initWithData:jsonData
                                                 encoding:NSUTF8StringEncoding];
    return strdup([jsonString UTF8String]);
  }
}

int SetVCP(unsigned int displayID, unsigned char featureCode,
           unsigned short value) {
  @autoreleasepool {
    io_service_t service = GetIOServicePort(displayID);
    if (!service) {
      return -1;
    }

    IOI2CRequest request;
    memset(&request, 0, sizeof(request));

    UInt8 data[7];
    data[0] = 0x51; // DDC/CI host address
    data[1] = 0x84; // Set VCP feature command
    data[2] = featureCode;
    data[3] = (value >> 8) & 0xFF; // High byte
    data[4] = value & 0xFF;        // Low byte

    // Calculation checksum
    UInt8 checksum = 0x6E; // 0x50 XOR 0x51 XOR 0x84
    checksum ^= featureCode;
    checksum ^= data[3];
    checksum ^= data[4];
    data[5] = checksum;

    request.sendAddress = 0x37 << 1; // DDC/CI address
    request.sendTransactionType = kIOI2CSimpleTransactionType;
    request.sendBuffer = (vm_address_t)data;
    request.sendBytes = 6;
    request.minReplyDelay = kDDCMinReplyDelay;

    IOItemCount busCount;
    IOReturn ret = IOFBGetI2CInterfaceCount(service, &busCount);
    for (IOItemCount bus = 0; bus < busCount; bus++) {
      io_service_t interface;
      ret = IOFBCopyI2CInterfaceForBus(service, bus, &interface);
      if (ret != kIOReturnSuccess)
        continue;
      IOI2CConnectRef connect;
      ret = IOI2CInterfaceOpen(interface, kNilOptions, &connect);
      IOObjectRelease(interface);

      if (ret == kIOReturnSuccess) {
        ret = IOI2CSendRequest(connect, kNilOptions, &request);
        IOI2CInterfaceClose(connect, kNilOptions);

        if (ret == kIOReturnSuccess) {
          IOObjectRelease(service);
          return 0;
        }
      }
    }
    IOObjectRelease(service);
    return -2;
  }
}

int GetVCP(unsigned int displayID, unsigned char featureCode,
           unsigned short *currentValue, unsigned short *maxValue) {
  @autoreleasepool {
    io_service_t service = GetIOServicePort(displayID);
    if (!service) {
      return -1;
    }

    // Send request
    UInt8 sendData[5];
    sendData[0] = 0x51;
    sendData[1] = 0x82; // Get VCP feature command
    sendData[2] = featureCode;
    sendData[3] = 0x6E ^ featureCode; // Checksum

    // Receive buffer
    UInt8 recvData[11];

    IOI2CRequest request;
    memset(&request, 0, sizeof(request));
    request.sendAddress = 0x37 << 1;
    request.sendTransactionType = kIOI2CSimpleTransactionType;
    request.sendBuffer = (vm_address_t)sendData;
    request.sendBytes = 4;
    request.replyAddress = 0x37 << 1;
    request.replyTransactionType = kIOI2CSimpleTransactionType;
    request.replyBuffer = (vm_address_t)recvData;
    request.replyBytes = sizeof(recvData);
    request.minReplyDelay = kDDCMinReplyDelay;

    IOItemCount busCount;
    IOReturn ret = IOFBGetI2CInterfaceCount(service, &busCount);

    for (IOItemCount bus = 0; bus < busCount; bus++) {
      io_service_t interface;
      ret = IOFBCopyI2CInterfaceForBus(service, bus, &interface);
      if (ret != kIOReturnSuccess)
        continue;

      IOI2CConnectRef connect;
      ret = IOI2CInterfaceOpen(interface, kNilOptions, &connect);
      IOObjectRelease(interface);

      if (ret == kIOReturnSuccess) {
        ret = IOI2CSendRequest(connect, kNilOptions, &request);
        IOI2CInterfaceClose(connect, kNilOptions);

        if (ret == kIOReturnSuccess && request.replyBytes >= 8) {
          // Parse response:
          // [addr][len][02][00][feature][type][max_h][max_l][cur_h][cur_l][checksum]
          if (recvData[2] == 0x02 && recvData[4] == featureCode) {
            *maxValue = (recvData[6] << 8) | recvData[7];
            *currentValue = (recvData[8] << 8) | recvData[9];
            IOObjectRelease(service);
            return 0;
          }
        }
      }
    }

    IOObjectRelease(service);
    return -2;
  }
}

void FreeString(char *str) {
  if (str) {
    free(str);
  }
}
