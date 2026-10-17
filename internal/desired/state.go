package desired

import "website.goodwin.vpn/plane/internal/reality"

type State struct {
	VLESS *VLESS `json:"vless,omitempty"`
	Hy2   *Hy2   `json:"hy2,omitempty"`
	TT    *TT    `json:"tt,omitempty"`
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

type TT struct {
	Port      int      `json:"port"`
	Hostname  string   `json:"hostname"`
	Advertise string   `json:"advertise"`
	Name      string   `json:"name,omitempty"`
	Cert      string   `json:"cert,omitempty"`
	Key       string   `json:"key,omitempty"`
	Users     []TTUser `json:"users"`
}

type TTUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type TTLink struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Link     string `json:"link"`
}

type ApplyResult struct {
	OK          bool     `json:"ok"`
	XrayListen  bool     `json:"xray_listen"`
	Hy2Listen   bool     `json:"hy2_listen"`
	TTListen    bool     `json:"tt_listen"`
	XrayVersion string   `json:"xray_version,omitempty"`
	Hy2Version  string   `json:"hy2_version,omitempty"`
	TTVersion   string   `json:"tt_version,omitempty"`
	TTLinks     []TTLink `json:"tt_links,omitempty"`
	Detail      string   `json:"detail,omitempty"`
}
