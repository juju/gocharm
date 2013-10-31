package runhook
import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
)

type handler struct {
	mu sync.Mutex
	currentVal string
}

func newHandler() http.Handler {
	return &handler{}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/val" {
		http.NotFound(w, req)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	switch req.Method {
	case "GET":
		fmt.Fprintf(w, "%s", h.currentVal)
	case "PUT":
		// This should have some security protection around it,
		// or we could use a separate channel between hook and
		// http server instead.
		data, err := ioutil.ReadAll(req.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusInternalServerError)
			return
		}
		h.currentVal = string(data)
	default:
		http.Error(w, "unsupported method", http.StatusBadRequest)
	}
}
