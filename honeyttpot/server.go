package honeyttpot

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

type Server interface {
	Name() string
	HandleError(*http.Request, http.ResponseWriter)
	HandleSuccess(*http.Request, http.ResponseWriter)
}

type Nginx struct {
	version string
	output  string
}

func NewNginx(version string, contents io.Reader) (*Nginx, error) {
	var all_contents []byte
	var all_error error
	if all_contents, all_error = io.ReadAll(contents); all_error != nil {
		return nil, all_error
	}

	return &Nginx{version: version, output: string(all_contents)}, nil
}

func (ngx *Nginx) Name() string {
	return fmt.Sprintf("nginx/%s", ngx.version)
}

func (ngx *Nginx) HandleError(*http.Request, http.ResponseWriter) {
}

func (ngx *Nginx) HandleSuccess(request *http.Request, response_writer http.ResponseWriter) {
	response_writer.Header().Add("Last-modified", time.Now().Format("Mon, 2 Jan 2006 15:04:05 MST"))
	response_writer.Write([]byte(ngx.output))
}
