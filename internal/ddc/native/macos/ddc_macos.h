#ifndef DDC_MACOS_H
#define DDC_MACOS_H

#ifdef __cplusplus
extern "C" {
#endif
// Returns a JSON string representing an array of detected monitors.
// The caller is responsible for freeing the returned char*.

char *GetMonitorsJSON();

// Returns a JSON string representing an array of detected monitors.
// The caller is responsible for freeing the returned char*.
int SetVCP(unsigned int displayID, unsigned char featureCOde,
           unsigned short value);

// Gets a VCP feature value from a specified display.
// Returns 0 on success, non-zero on error.
int GetVCP(unsigned int displayID, unsigned char featureCode,
           unsigned short *value, unsigned short *maxValue);

// Frees memory allocated by GetMonitorsJSON.
void FreeString(char *str);

#ifdef __cplusplus
}
#endif

#endif // DDC_MACOS_H
