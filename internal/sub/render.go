package sub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"website.goodwin.vpn/plane/internal/geo"
	"website.goodwin.vpn/plane/internal/stack"
)

type Headers struct {
	Title         string
	IntervalHours int
	Upload        int64
	Download      int64
	Total         int64
	ExpireUnix    int64
	DeviceLimit   int
	DeviceCount   int // optional; 0 = omit (panel does not invent live sessions)
}

type User struct {
	DisplayName    string
	VlessUUID      string
	Hy2Password    string
	TTUser         string
	TTPassword     string
	TTLink         string
	TrojanPassword string
	SsPassword     string
	Upload         int64
	Download       int64
	Total          int64
	Expire         *time.Time
	Status         string
}

type NodeLine struct {
	Name     string
	Host     string
	Family   string
	Port     int
	Reality  *Reality
	Security string // tls for non-reality vless/vmess/trojan
	TLSSni   string
	Network  string
	Path     string
	WSHost   string
	Method   string // shadowsocks
	Hy2SNI   string
	TTLink   string
}

type Reality struct {
	SNI         string
	PublicKey   string
	ShortID     string
	Flow        string
	FP          string
	Network     string
	ServiceName string
}

type Result struct {
	Body    string
	Headers Headers
}

func Render(user User, lines []NodeLine) (Result, error) {
	if user.Status != "" && user.Status != "active" {
		return Result{}, fmt.Errorf("user is not active")
	}
	var body []string
	for _, line := range lines {
		s, err := shareLink(user, line)
		if err != nil {
			return Result{}, err
		}
		if s != "" {
			body = append(body, s)
		}
	}
	if len(body) == 0 {
		return Result{}, fmt.Errorf("no share links")
	}
	h := Headers{
		Title:         strings.TrimSpace(user.DisplayName),
		IntervalHours: 24,
		Upload:        user.Upload,
		Download:      user.Download,
		Total:         user.Total,
	}
	if h.Title == "" {
		h.Title = "Goodwin"
	}
	if user.Expire != nil && !user.Expire.IsZero() {
		h.ExpireUnix = user.Expire.UTC().Unix()
	}
	return Result{
		Body:    strings.Join(body, "\n") + "\n",
		Headers: h,
	}, nil
}

func shareLink(user User, line NodeLine) (string, error) {
	host := strings.TrimSpace(line.Host)
	if host == "" {
		return "", fmt.Errorf("node %q missing host", line.Name)
	}
	name := strings.TrimSpace(line.Name)
	if name == "" {
		name = host
	}
	frag := url.QueryEscape(name)
	port := line.Port
	if port <= 0 {
		port = 443
	}
	switch line.Family {
	case stack.FamilyVLESS:
		if strings.TrimSpace(user.VlessUUID) == "" {
			return "", nil
		}
		q := url.Values{}
		q.Set("encryption", "none")
		network := line.Network
		if network == "" {
			network = "tcp"
			if line.Reality != nil && line.Reality.PublicKey != "" {
				network = "grpc"
				if line.Reality.Network != "" {
					network = line.Reality.Network
				}
			}
		}
		q.Set("type", network)
		if line.Reality != nil && line.Reality.PublicKey != "" {
			q.Set("security", "reality")
			q.Set("pbk", line.Reality.PublicKey)
			q.Set("sid", line.Reality.ShortID)
			sni := line.Reality.SNI
			if sni == "" {
				sni = host
			}
			q.Set("sni", sni)
			fp := line.Reality.FP
			if fp == "" {
				fp = "chrome"
			}
			q.Set("fp", fp)
			if network == "grpc" {
				svc := line.Reality.ServiceName
				if svc == "" {
					svc = "goodwin"
				}
				q.Set("serviceName", svc)
				q.Set("mode", "gun")
			}
			if line.Reality.Flow != "" {
				q.Set("flow", line.Reality.Flow)
			}
		} else if line.Security == "tls" {
			q.Set("security", "tls")
			sni := line.TLSSni
			if sni == "" {
				sni = host
			}
			q.Set("sni", sni)
			q.Set("fp", "chrome")
		} else {
			q.Set("security", "none")
		}
		if network == "ws" {
			path := line.Path
			if path == "" {
				path = stack.WSPath
			}
			q.Set("path", path)
			wsHost := line.WSHost
			if wsHost == "" && line.Reality != nil && line.Reality.SNI != "" {
				wsHost = line.Reality.SNI
			}
			if wsHost == "" {
				wsHost = line.TLSSni
			}
			if wsHost != "" {
				q.Set("host", wsHost)
			}
		}
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s", user.VlessUUID, host, port, q.Encode(), frag), nil
	case stack.FamilyVMess:
		if strings.TrimSpace(user.VlessUUID) == "" {
			return "", nil
		}
		network := line.Network
		if network == "" {
			network = "ws"
		}
		sni := line.TLSSni
		if sni == "" {
			sni = host
		}
		path := line.Path
		if path == "" {
			path = stack.WSPath
		}
		obj := map[string]any{
			"v":    "2",
			"ps":   name,
			"add":  host,
			"port": port,
			"id":   user.VlessUUID,
			"aid":  0,
			"scy":  "auto",
			"net":  network,
			"type": "none",
			"host": sni,
			"path": path,
			"tls":  "tls",
			"sni":  sni,
			"fp":   "chrome",
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return "", err
		}
		return "vmess://" + base64.StdEncoding.EncodeToString(raw), nil
	case stack.FamilyTrojan:
		if strings.TrimSpace(user.TrojanPassword) == "" {
			return "", nil
		}
		q := url.Values{}
		q.Set("security", "tls")
		sni := line.TLSSni
		if sni == "" {
			sni = host
		}
		q.Set("sni", sni)
		q.Set("fp", "chrome")
		q.Set("type", "tcp")
		pass := url.PathEscape(user.TrojanPassword)
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s", pass, host, port, q.Encode(), frag), nil
	case stack.FamilySS:
		if strings.TrimSpace(user.SsPassword) == "" {
			return "", nil
		}
		method := line.Method
		if method == "" {
			method = stack.DefaultSSMethod
		}
		userinfo := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(
			[]byte(method + ":" + user.SsPassword),
		)
		return fmt.Sprintf("ss://%s@%s:%d#%s", userinfo, host, port, frag), nil
	case stack.FamilyHy2:
		if strings.TrimSpace(user.Hy2Password) == "" {
			return "", nil
		}
		q := url.Values{}
		sni := line.Hy2SNI
		if sni == "" {
			sni = host
		}
		q.Set("sni", sni)
		pass := url.PathEscape(user.Hy2Password)
		return fmt.Sprintf("hysteria2://%s@%s:%d?%s#%s", pass, host, port, q.Encode(), frag), nil
	case stack.FamilyTT:
		if link := strings.TrimSpace(line.TTLink); link != "" {
			return link, nil
		}
		if strings.TrimSpace(user.TTLink) != "" {
			return strings.TrimSpace(user.TTLink), nil
		}
		return "", nil
	default:
		return "", fmt.Errorf("unknown family %q", line.Family)
	}
}

func WriteHeaders(dst map[string]string, h Headers) {
	if h.Title != "" {
		dst["profile-title"] = h.Title
	}
	if h.IntervalHours > 0 {
		dst["profile-update-interval"] = fmt.Sprintf("%d", h.IntervalHours)
	}
	userinfo := fmt.Sprintf(
		"upload=%d; download=%d; total=%d; expire=%d",
		h.Upload, h.Download, h.Total, h.ExpireUnix,
	)
	if h.DeviceLimit > 0 {
		userinfo += fmt.Sprintf("; device-limit=%d", h.DeviceLimit)
	}
	if h.DeviceCount > 0 {
		userinfo += fmt.Sprintf("; device-count=%d", h.DeviceCount)
	}
	dst["subscription-userinfo"] = userinfo
}

// PublicHTTPSOrigin is the Goodwin service base from PUBLIC_SUB_BASE.
// Empty when the value is not https (local http://127.0.0.1:8080).
func PublicHTTPSOrigin(publicSubBase string) string {
	u, err := url.Parse(strings.TrimSpace(publicSubBase))
	if err != nil || u.User != nil {
		return ""
	}
	if !strings.EqualFold(u.Scheme, "https") || strings.TrimSpace(u.Host) == "" {
		return ""
	}
	return "https://" + u.Host
}

// ServiceHeader is the Goodwin-VPN value advertised on GET /sub/{token}.
func ServiceHeader(publicSubBase string) string {
	origin := PublicHTTPSOrigin(publicSubBase)
	if origin == "" {
		return ""
	}
	return fmt.Sprintf(`v1; base="%s"`, origin)
}

// ServiceDocument is GET /gw/v1/service. Unknown client features are ignored.
type ServiceDocument struct {
	Protocol string   `json:"protocol"`
	Version  int      `json:"version"`
	Name     string   `json:"name"`
	Privacy  string   `json:"privacy,omitempty"`
	Support  string   `json:"support,omitempty"`
	Features []string `json:"features"`
}

func ServiceDocumentFor(publicSubBase string) ServiceDocument {
	doc := ServiceDocument{
		Protocol: "goodwin-vpn",
		Version:  1,
		Name:     "Goodwin VPN",
		Features: geo.ServiceFeatures(),
	}
	if origin := PublicHTTPSOrigin(publicSubBase); origin != "" {
		doc.Privacy = origin + "/privacy"
		doc.Support = origin + "/support"
	}
	return doc
}
