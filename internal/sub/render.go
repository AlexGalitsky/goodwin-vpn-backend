package sub

import (
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
}

type User struct {
	DisplayName string
	VlessUUID   string
	Hy2Password string
	TTUser      string
	TTPassword  string
	TTLink      string
	Upload      int64
	Download    int64
	Total       int64
	Expire      *time.Time
	Status      string
}

type NodeLine struct {
	Name    string
	Host    string
	Family  string
	Port    int
	Reality *Reality
	Hy2SNI  string
	TTLink  string
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
	switch line.Family {
	case stack.FamilyVLESS:
		if strings.TrimSpace(user.VlessUUID) == "" {
			return "", nil
		}
		port := line.Port
		if port <= 0 {
			port = 443
		}
		q := url.Values{}
		q.Set("encryption", "none")
		network := "tcp"
		if line.Reality != nil && line.Reality.PublicKey != "" {
			network = "grpc"
			if line.Reality.Network != "" {
				network = line.Reality.Network
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
		} else {
			q.Set("security", "none")
		}
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s", user.VlessUUID, host, port, q.Encode(), frag), nil
	case stack.FamilyHy2:
		if strings.TrimSpace(user.Hy2Password) == "" {
			return "", nil
		}
		port := line.Port
		if port <= 0 {
			port = 443
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
	dst["subscription-userinfo"] = fmt.Sprintf(
		"upload=%d; download=%d; total=%d; expire=%d",
		h.Upload, h.Download, h.Total, h.ExpireUnix,
	)
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
	}
	return doc
}
