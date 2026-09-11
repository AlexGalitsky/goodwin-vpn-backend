package desired

import "website.goodwin.vpn/plane/internal/reality"

type State struct {
	VLESS *VLESS `json:"vless,omitempty"`
}

type VLESS struct {
	Port    int             `json:"port"`
	Reality reality.Keys    `json:"reality"`
	Clients []VLESSClient   `json:"clients"`
}

type VLESSClient struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Flow  string `json:"flow"`
}

type ApplyResult struct {
	OK         bool   `json:"ok"`
	XrayListen bool   `json:"xray_listen"`
	XrayVersion string `json:"xray_version,omitempty"`
	Detail     string `json:"detail,omitempty"`
}
