package osystems

import (
	"fmt"
	"log/slog"
	"runtime"
)

type OSProp struct {
	osType  string
	command string
	params  string
}

func Init(osProp OSProp) (OSProp, error) {
	slog.Info("Initializing OS module", "osprop", osProp)
	if osProp.osType != runtime.GOOS { // if the user has passed mac and the current OS is windows - HALT!
		slog.Error("OsSystemsInit:: The osProp settings do not match the current OS, hence will not proceed ahead", "currentOS", runtime.GOOS, "passedOS", osProp.osType)
		return osProp, fmt.Errorf("The %s which was passed, do not match your current OS (%s)", osProp.osType, runtime.GOOS)
	}
	slog.Info("OsSystemsInit:: We can run the script on this machine!")

	return osProp, nil
}
