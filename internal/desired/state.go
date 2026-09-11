package desired

import "website.goodwin.vpn/plane/internal/reality"

type State struct {
	VLESS *VLESS `json:"vless,omitempty"`
	Hy2   *Hy2   `json:"hy2,omitempty"`
}

type VLESS struct {
	Port        int           `json:"port"`
	Network     string        `json:"network,omitempty"`
	ServiceName string        `json:"service_name,omitempty"`
	Reality     reality.Keys  `json:"reality"`
	Clients     []VLESSClient `json:"clients"`
}

type VLESSClient struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Flow  string `json:"flow"`
}

type Hy2 struct {
	Port     int       `json:"port"`
	Hostname string    `json:"hostname"`
	Cert     string    `json:"cert,omitempty"`
	Key      string    `json:"key,omitempty"`
	Users    []Hy2User `json:"users"`
}

type Hy2User struct {
	ID       string `json:"id"`
	Password string `json:"password"`
}

type ApplyResult struct {
	OK          bool   `json:"ok"`
	XrayListen  bool   `json:"xray_listen"`
	Hy2Listen   bool   `json:"hy2_listen"`
	XrayVersion string `json:"xray_version,omitempty"`
	Hy2Version  string `json:"hy2_version,omitempty"`
	Detail      string `json:"detail,omitempty"`
}
