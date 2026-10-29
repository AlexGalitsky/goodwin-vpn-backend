package desired

import "website.goodwin.vpn/plane/internal/reality"

type State struct {
	VLESS *VLESS `json:"vless,omitempty"` // Xray multi-inbound (legacy key name)
	Hy2   *Hy2   `json:"hy2,omitempty"`
	TT    *TT    `json:"tt,omitempty"`
}

// VLESS is the Xray desired block (vless/vmess/trojan/ss inbounds).
type VLESS struct {
	Port        int           `json:"port"` // primary / legacy
	Network     string        `json:"network,omitempty"`
	ServiceName string        `json:"service_name,omitempty"`
	Reality     reality.Keys  `json:"reality"`
	Hostname    string        `json:"hostname,omitempty"` // LE SAN for tls
	Cert        string        `json:"cert,omitempty"`
	Key         string        `json:"key,omitempty"`
	Clients     []VLESSClient `json:"clients"` // vless + vmess (uuid)
	TrojanUsers []TrojanUser  `json:"trojan_users,omitempty"`
	SSUsers     []SSUser      `json:"ss_users,omitempty"`
	Inbounds    []XrayInbound `json:"inbounds,omitempty"`
}

type XrayInbound struct {
	Protocol    string `json:"protocol"` // vless|vmess|trojan|shadowsocks
	Security    string `json:"security,omitempty"`
	Network     string `json:"network,omitempty"`
	Port        int    `json:"port"`
	Method      string `json:"method,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	Flow        string `json:"flow,omitempty"`
	Path        string `json:"path,omitempty"`
	Host        string `json:"host,omitempty"`
	Label       string `json:"label,omitempty"`
}

type VLESSClient struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Flow  string `json:"flow"`
}

type TrojanUser struct {
	ID       string `json:"id"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

type SSUser struct {
	ID       string `json:"id"`
	Password string `json:"password"`
	Email    string `json:"email"`
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
