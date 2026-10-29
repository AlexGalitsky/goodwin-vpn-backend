package stack

import (
	"fmt"
	"strings"
)

const (
	FamilyVLESS     = "vless"
	FamilyVMess     = "vmess"
	FamilyTrojan    = "trojan"
	FamilySS        = "ss"
	FamilyHy2       = "hy2"
	FamilyTT        = "tt"
	VisionFlow      = "xtls-rprx-vision"
	DefaultSSMethod = "aes-256-gcm"
	WSPath          = "/goodwin"
	GRPCService     = "goodwin"
)

type XrayListen struct {
	Protocol    string `json:"protocol"`           // vless|vmess|trojan|shadowsocks
	Security    string `json:"security,omitempty"` // reality|tls
	Network     string `json:"network,omitempty"`  // tcp|ws|grpc
	Port        int    `json:"port"`
	Method      string `json:"method,omitempty"` // ss
	ServiceName string `json:"service_name,omitempty"`
	Flow        string `json:"flow,omitempty"`
	Path        string `json:"path,omitempty"`
	Host        string `json:"host,omitempty"`
	Label       string `json:"label,omitempty"`
}

type Ports struct {
	VlessTCP int          `json:"vless_tcp,omitempty"`
	Xray     []XrayListen `json:"xray,omitempty"`
	Vless    []XrayListen `json:"vless,omitempty"` // alias for older drafts
	Hy2UDP   int          `json:"hy2_udp,omitempty"`
	TT       int          `json:"tt,omitempty"`
}

type Spec struct {
	Families []string `json:"families"`
	Ports    Ports    `json:"ports"`
	Preset   string   `json:"preset,omitempty"`
}

func DefaultPorts(preset string) Ports {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "tt-first":
		return Ports{VlessTCP: 8443, Hy2UDP: 8443, TT: 443}
	case "stealth":
		return Ports{VlessTCP: 443}
	case "hy2":
		return Ports{Hy2UDP: 443}
	case "tt":
		return Ports{TT: 443}
	case "stealth-dual":
		return Ports{Xray: []XrayListen{
			vlessRealityGRPC(443),
			vlessRealityVision(9443),
		}}
	case "stealth-full":
		return Ports{Xray: []XrayListen{
			vlessRealityGRPC(443),
			vlessRealityVision(9443),
			vlessRealityWS(2083),
		}}
	case "max-dual":
		return Ports{
			Xray: []XrayListen{
				vlessRealityGRPC(443),
				vlessRealityVision(9443),
			},
			Hy2UDP: 443,
			TT:     8443,
		}
	case "max-full":
		return Ports{
			Xray: []XrayListen{
				vlessRealityGRPC(443),
				vlessRealityVision(9443),
				vlessRealityWS(2083),
			},
			Hy2UDP: 443,
			TT:     8443,
		}
	case "review":
		return Ports{
			Xray: []XrayListen{
				vlessRealityGRPC(443),
				vlessRealityVision(9443),
				vlessRealityWS(2083),
				vlessTLS(10443),
				vmessTLSWS(2096),
				trojanTLS(10444),
				shadowsocks(8388),
			},
			Hy2UDP: 443,
			TT:     8443,
		}
	default: // max
		return Ports{VlessTCP: 443, Hy2UDP: 443, TT: 8443}
	}
}

func FamiliesForPreset(preset string) []string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "stealth", "stealth-dual", "stealth-full":
		return []string{FamilyVLESS}
	case "hy2":
		return []string{FamilyHy2}
	case "tt":
		return []string{FamilyTT}
	case "review":
		return []string{FamilyVLESS, FamilyVMess, FamilyTrojan, FamilySS, FamilyHy2, FamilyTT}
	case "tt-first", "max", "max-stack", "max-dual", "max-full", "":
		return []string{FamilyVLESS, FamilyHy2, FamilyTT}
	default:
		return []string{FamilyVLESS, FamilyHy2, FamilyTT}
	}
}

func vlessRealityGRPC(port int) XrayListen {
	return XrayListen{
		Protocol: FamilyVLESS, Security: "reality", Network: "grpc", Port: port,
		ServiceName: GRPCService, Label: "grpc",
	}
}

func vlessRealityVision(port int) XrayListen {
	return XrayListen{
		Protocol: FamilyVLESS, Security: "reality", Network: "tcp", Port: port,
		Flow: VisionFlow, Label: "tcp",
	}
}

func vlessRealityWS(port int) XrayListen {
	return XrayListen{
		Protocol: FamilyVLESS, Security: "reality", Network: "ws", Port: port,
		Path: WSPath, Label: "ws",
	}
}

func vlessTLS(port int) XrayListen {
	return XrayListen{
		Protocol: FamilyVLESS, Security: "tls", Network: "tcp", Port: port,
		Label: "vless-tls",
	}
}

func vmessTLSWS(port int) XrayListen {
	return XrayListen{
		Protocol: FamilyVMess, Security: "tls", Network: "ws", Port: port,
		Path: WSPath, Label: "vmess",
	}
}

func trojanTLS(port int) XrayListen {
	return XrayListen{
		Protocol: FamilyTrojan, Security: "tls", Network: "tcp", Port: port,
		Label: "trojan",
	}
}

func shadowsocks(port int) XrayListen {
	return XrayListen{
		Protocol: "shadowsocks", Network: "tcp", Port: port,
		Method: DefaultSSMethod, Label: "ss",
	}
}

// ResolveXray returns configured Xray listens. Legacy vless_tcp → one gRPC REALITY inbound.
func ResolveXray(ports Ports) []XrayListen {
	if len(ports.Xray) > 0 {
		return normalizeListens(ports.Xray)
	}
	if len(ports.Vless) > 0 {
		return normalizeListens(ports.Vless)
	}
	port := ports.VlessTCP
	if port <= 0 {
		return nil
	}
	return []XrayListen{vlessRealityGRPC(port)}
}

func normalizeListens(in []XrayListen) []XrayListen {
	out := make([]XrayListen, 0, len(in))
	for _, l := range in {
		l.Protocol = strings.ToLower(strings.TrimSpace(l.Protocol))
		if l.Protocol == "ss" {
			l.Protocol = "shadowsocks"
		}
		l.Security = strings.ToLower(strings.TrimSpace(l.Security))
		l.Network = strings.ToLower(strings.TrimSpace(l.Network))
		if l.Network == "" {
			l.Network = "tcp"
		}
		if l.Protocol == FamilyVLESS && l.Security == "" {
			l.Security = "reality"
		}
		if l.Protocol == FamilyVLESS && l.Network == "grpc" && l.ServiceName == "" {
			l.ServiceName = GRPCService
		}
		if l.Protocol == "shadowsocks" && l.Method == "" {
			l.Method = DefaultSSMethod
		}
		if l.Label == "" {
			l.Label = defaultLabel(l)
		}
		out = append(out, l)
	}
	return out
}

func defaultLabel(l XrayListen) string {
	switch l.Protocol {
	case "shadowsocks":
		return "ss"
	case FamilyVMess:
		return "vmess"
	case FamilyTrojan:
		return "trojan"
	case FamilyVLESS:
		if l.Security == "tls" {
			return "vless-tls"
		}
		return l.Network
	default:
		return l.Protocol
	}
}

// FamilyForListen maps an Xray listen to a stack family for AppliedFamilies / groups.
func FamilyForListen(l XrayListen) string {
	switch strings.ToLower(l.Protocol) {
	case FamilyVMess:
		return FamilyVMess
	case FamilyTrojan:
		return FamilyTrojan
	case "shadowsocks", FamilySS:
		return FamilySS
	default:
		return FamilyVLESS
	}
}

func NeedsTLSCert(ports Ports) bool {
	for _, l := range ResolveXray(ports) {
		if l.Security == "tls" || l.Protocol == FamilyTrojan || l.Protocol == FamilyVMess {
			return true
		}
	}
	return false
}

func NormalizeFamilies(in []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		f := strings.ToLower(strings.TrimSpace(raw))
		switch f {
		case FamilyVLESS, FamilyVMess, FamilyTrojan, FamilySS, FamilyHy2, FamilyTT:
		case "shadowsocks":
			f = FamilySS
		default:
			return nil, fmt.Errorf("unknown family %q", raw)
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("select at least one family")
	}
	return out, nil
}

func HasFamily(families []string, want string) bool {
	for _, f := range families {
		if f == want {
			return true
		}
	}
	return false
}

func NeedsHostname(families []string) bool {
	return HasFamily(families, FamilyHy2) || HasFamily(families, FamilyTT) ||
		HasFamily(families, FamilyVMess) || HasFamily(families, FamilyTrojan)
}

// DetectPreset maps stored families+ports back to a named preset.
func DetectPreset(families []string, ports Ports) string {
	vless := HasFamily(families, FamilyVLESS)
	hy2 := HasFamily(families, FamilyHy2)
	tt := HasFamily(families, FamilyTT)
	xray := ResolveXray(ports)
	n := len(xray)
	hasReviewExtras := HasFamily(families, FamilyVMess) || HasFamily(families, FamilyTrojan) || HasFamily(families, FamilySS)

	if hasReviewExtras && n >= 7 {
		return "review"
	}
	if vless && hy2 && tt && n == 3 {
		return "max-full"
	}
	if vless && hy2 && tt && n == 2 {
		return "max-dual"
	}
	if vless && !hy2 && !tt && n == 3 {
		return "stealth-full"
	}
	if vless && !hy2 && !tt && n == 2 {
		return "stealth-dual"
	}
	switch {
	case vless && !hy2 && !tt && n <= 1:
		return "stealth"
	case hy2 && !vless && !tt:
		return "hy2"
	case tt && !vless && !hy2:
		return "tt"
	case vless && hy2 && tt && ports.TT == 443 && (ports.VlessTCP == 8443 || ports.Hy2UDP == 8443):
		return "tt-first"
	default:
		return "max"
	}
}

func Validate(spec Spec) error {
	vless := HasFamily(spec.Families, FamilyVLESS)
	vmess := HasFamily(spec.Families, FamilyVMess)
	trojan := HasFamily(spec.Families, FamilyTrojan)
	ss := HasFamily(spec.Families, FamilySS)
	hy2 := HasFamily(spec.Families, FamilyHy2)
	tt := HasFamily(spec.Families, FamilyTT)
	if !vless && !vmess && !trojan && !ss && !hy2 && !tt {
		return fmt.Errorf("select at least one family")
	}

	listens := ResolveXray(spec.Ports)
	wantXray := vless || vmess || trojan || ss
	if wantXray && len(listens) == 0 {
		return fmt.Errorf("xray family selected but no listens / vless_tcp")
	}

	seenPort := map[int]string{}
	markTCP := func(port int, who string) error {
		if port <= 0 {
			return fmt.Errorf("%s needs a TCP port", who)
		}
		if prev, ok := seenPort[port]; ok {
			return fmt.Errorf("%s and %s share TCP port %d — split them", prev, who, port)
		}
		seenPort[port] = who
		return nil
	}

	for _, l := range listens {
		fam := FamilyForListen(l)
		if !HasFamily(spec.Families, fam) {
			return fmt.Errorf("listen %s not allowed by families", l.Label)
		}
		switch l.Protocol {
		case FamilyVLESS, FamilyVMess, FamilyTrojan, "shadowsocks":
		default:
			return fmt.Errorf("unknown xray protocol %q", l.Protocol)
		}
		switch l.Network {
		case "tcp", "ws", "grpc":
		default:
			return fmt.Errorf("unsupported network %q", l.Network)
		}
		if l.Flow != "" {
			if l.Protocol != FamilyVLESS || l.Security != "reality" || l.Network != "tcp" {
				return fmt.Errorf("vision flow only on vless reality tcp")
			}
		}
		if l.Protocol == FamilyVLESS && l.Network == "grpc" && l.Flow != "" {
			return fmt.Errorf("grpc must not set vision flow")
		}
		if l.Protocol == "shadowsocks" {
			if strings.TrimSpace(l.Method) == "" {
				return fmt.Errorf("shadowsocks needs a method")
			}
		} else if l.Security != "reality" && l.Security != "tls" {
			return fmt.Errorf("%s needs security reality|tls", l.Protocol)
		}
		if err := markTCP(l.Port, "xray:"+l.Label); err != nil {
			return err
		}
	}

	if hy2 && spec.Ports.Hy2UDP <= 0 {
		return fmt.Errorf("hy2 needs a UDP port")
	}
	if tt && spec.Ports.TT <= 0 {
		return fmt.Errorf("trusttunnel needs a TCP+UDP port")
	}
	if tt {
		if err := markTCP(spec.Ports.TT, "trusttunnel"); err != nil {
			return err
		}
		if hy2 && spec.Ports.Hy2UDP == spec.Ports.TT {
			return fmt.Errorf("hy2 UDP and trusttunnel share port %d — split them", spec.Ports.TT)
		}
	}
	return nil
}
