package sloth

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
)

// GetSupported is the interface that provides the Get
// method a resource must support to receive HTTP GETs.
type GetSupported interface {
	Get(*http.Request) (int, interface{}, http.Header)
}

// PostSupported is the interface that provides the Post
// method a resource must support to receive HTTP POSTs.
type PostSupported interface {
	Post(*http.Request) (int, interface{}, http.Header)
}

// PutSupported is the interface that provides the Put
// method a resource must support to receive HTTP PUTs.
type PutSupported interface {
	Put(*http.Request) (int, interface{}, http.Header)
}

// DeleteSupported is the interface that provides the Delete
// method a resource must support to receive HTTP DELETEs.
type DeleteSupported interface {
	Delete(*http.Request) (int, interface{}, http.Header)
}

// HeadSupported is the interface that provides the Head
// method a resource must support to receive HTTP HEADs.
type HeadSupported interface {
	Head(*http.Request) (int, interface{}, http.Header)
}

// PatchSupported is the interface that provides the Patch
// method a resource must support to receive HTTP PATCHs.
type PatchSupported interface {
	Patch(*http.Request) (int, interface{}, http.Header)
}

// APIMux interface for arbitrary muxer support (like http.ServeMux).
type APIMux interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) *mux.Route
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// An API manages a group of resources by routing requests
// to the correct method on a matching resource and marshalling
// the returned data to JSON for the HTTP response.
//
// You can instantiate multiple APIs on separate ports. Each API
// will manage its own set of resources.
type API struct {
	mux                APIMux
	muxInitialized     bool
	defaultParseForm   bool
	defaultContentType string
}

// NewAPI allocates and returns a new API.
func NewAPI() *API {
	return &API{defaultParseForm: true, defaultContentType: "application/json"}
}

func (api *API) requestHandler(resource interface{}) http.HandlerFunc {
	return func(rw http.ResponseWriter, request *http.Request) {

		if api.defaultParseForm && request.ParseForm() != nil {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		var handler func(*http.Request) (int, interface{}, http.Header)

		switch request.Method {
		case "GET":
			if resource, ok := resource.(GetSupported); ok {
				handler = resource.Get
			}
		case "POST":
			if resource, ok := resource.(PostSupported); ok {
				handler = resource.Post
			}
		case "PUT":
			if resource, ok := resource.(PutSupported); ok {
				handler = resource.Put
			}
		case "DELETE":
			if resource, ok := resource.(DeleteSupported); ok {
				handler = resource.Delete
			}
		case "HEAD":
			if resource, ok := resource.(HeadSupported); ok {
				handler = resource.Head
			}
		case "PATCH":
			if resource, ok := resource.(PatchSupported); ok {
				handler = resource.Patch
			}
		}

		if handler == nil {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		code, data, header := handler(request)

		var content []byte
		var err error

		switch data.(type) {
		case string:
			content = []byte(data.(string))
		case []byte:
			content = data.([]byte)
		default:
			// Encode JSON.
			content, err = json.MarshalIndent(data, "", "  ")
			if err == nil && api.defaultContentType != "" {
				if header == nil {
					header = http.Header{"Content-Type": {api.defaultContentType}}
				} else if header.Get("Content-Type") == "" {
					header.Set("Content-Type", api.defaultContentType)
				}
			}
		}

		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		for name, values := range header {
			for _, value := range values {
				rw.Header().Add(name, value)
			}
		}
		rw.WriteHeader(code)
		rw.Write(content)
	}
}

// Mux returns the muxer used by an API. If a ServeMux does not
// yet exist, a new *http.ServeMux will be created and returned.
func (api *API) Mux() APIMux {
	if api.muxInitialized {
		return api.mux
	}
	api.mux = mux.NewRouter()
	api.muxInitialized = true
	return api.mux
}

// SetMux sets the muxer to use by an API. A muxer needs to
// implement the APIMux interface (eg. http.ServeMux).
func (api *API) SetMux(mux APIMux) error {
	if api.muxInitialized {
		return errors.New("You cannot set a muxer when already initialized.")
	}
	api.mux = mux
	api.muxInitialized = true
	return nil
}

// SetDefaultContentType sets the content type response header value
// which is set when a handler did not set it. You can set the default
// content type to "" to set no content type in that case.
func (api *API) SetDefaultContentType(ct string) {
	api.defaultContentType = ct
}

// SetDefaultParseForm controls if incoming requests automatically parse
// form data using request.ParseForm.
func (api *API) SetDefaultParseForm(defaultParseForm bool) {
	api.defaultParseForm = defaultParseForm
}

// AddResource adds a new resource to an API. The API will route
// requests that match one of the given paths to the matching HTTP
// method on the resource.
func (api *API) AddResource(resource interface{}, paths ...string) {
	for _, path := range paths {
		api.Mux().HandleFunc(path, api.requestHandler(resource))
	}
}

// AddResourceWithWrapper behaves exactly like AddResource but wraps
// the generated handler function with a give wrapper function to allow
// to hook in Gzip support and similar.
func (api *API) AddResourceWithWrapper(resource interface{}, wrapper func(handler http.HandlerFunc) http.HandlerFunc, paths ...string) {
	for _, path := range paths {
		api.Mux().HandleFunc(path, wrapper(api.requestHandler(resource)))
	}
}

// Start causes the API to begin serving requests on the given port.
func (api *API) Start(port int) error {
	if !api.muxInitialized {
		return errors.New("You must add at least one resource to this API.")
	}
	portString := fmt.Sprintf(":%d", port)
	return http.ListenAndServe(portString, api.Mux())
}
