// Package deploy holds the files degoploy setup installs on a server.
package deploy

import _ "embed"

// Unit is the systemd unit, installed as /etc/systemd/system/degoploy.service.
//
//go:embed degoploy.service
var Unit []byte

// ConfigTemplate is the text/template for a new /etc/degoploy/degoploy.yaml.
//
//go:embed degoploy.yaml.tmpl
var ConfigTemplate string
