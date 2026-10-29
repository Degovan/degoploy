package mail

import "os"

// osHostname is a variable so tests can pin the EHLO name.
var osHostname = os.Hostname
