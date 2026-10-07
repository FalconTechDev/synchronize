package syn

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// The dashboard can pull and push, which makes two of its endpoints worth
// guarding even though it listens on localhost.
//
// A page on any website can post a form to http://localhost:7391/api/push
// without the user knowing. The browser sends it, the request arrives looking
// ordinary, and somebody's work gets published. Requiring POST does not stop
// it: a plain HTML form is a POST.
//
// Browsers do say where a request came from, and that is enough. Anything
// that is not a browser - curl, a script, the dashboard's own page - sends
// neither header and is let through unchanged.
func sameOriginOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
			switch site {
			case "same-origin", "none":
			default:
				http.Error(w, "this endpoint is not open to other sites", http.StatusForbidden)
				return
			}
		}
		if origin := r.Header.Get("Origin"); origin != "" && !sameHost(origin, r.Host) {
			http.Error(w, "this endpoint is not open to other sites", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func sameHost(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, host)
}

// listensBeyondThisMachine reports whether the address would accept
// connections from the network rather than only from here. The README
// suggests `--addr :8080` for changing the port, and that form listens
// everywhere - worth saying out loud, since the dashboard can push.
func listensBeyondThisMachine(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return true // ":8080" means every interface
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback() && !ip.IsUnspecified() || ip.IsUnspecified()
	}
	return !strings.EqualFold(host, "localhost")
}
