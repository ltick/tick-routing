package routing

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRouterNotFound(t *testing.T) {
	r := New(context.Background())
	h := func(c *Context) error {
		fmt.Fprint(c.ResponseWriter, "ok")
		return nil
	}
	r.Get("/users", h)
	r.Post("/users", h)
	r.NotFound(MethodNotAllowedHandler, NotFoundHandler)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "ok", res.Body.String(), "response body")
	assert.Equal(t, http.StatusOK, res.Code, "HTTP status code")

	res = httptest.NewRecorder()
	req, _ = http.NewRequest("PUT", "/users", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "GET, OPTIONS, POST", res.Header().Get("Allow"), "Allow header")
	assert.Equal(t, http.StatusMethodNotAllowed, res.Code, "HTTP status code")

	res = httptest.NewRecorder()
	req, _ = http.NewRequest("OPTIONS", "/users", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "GET, OPTIONS, POST", res.Header().Get("Allow"), "Allow header")
	assert.Equal(t, http.StatusOK, res.Code, "HTTP status code")

	res = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/users/", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "", res.Header().Get("Allow"), "Allow header")
	assert.Equal(t, http.StatusNotFound, res.Code, "HTTP status code")

	r.IgnoreTrailingSlash = true
	res = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/users/", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "ok", res.Body.String(), "response body")
	assert.Equal(t, http.StatusOK, res.Code, "HTTP status code")
}

func TestRouterUse(t *testing.T) {
	r := New(context.Background())
	assert.Equal(t, 2, len(r.notFoundHandlers))
	r.AppendStartupHandler(NotFoundHandler)
	assert.Equal(t, 3, len(r.notFoundHandlers))
}

func TestRouterRoute(t *testing.T) {
	r := New(context.Background())
	r.Get("/users").Name("users")
	assert.NotNil(t, r.Route("users"))
	assert.Nil(t, r.Route("users2"))
}

func TestRouterAdd(t *testing.T) {
	r := New(context.Background())
	assert.Equal(t, 0, r.maxParams)
	r.add("GET", "/users/<id>", nil)
	assert.Equal(t, 1, r.maxParams)
}

func TestRouterFind(t *testing.T) {
	r := New(context.Background())
	r.add("GET", "/users/<id>", []Handler{NotFoundHandler})
	handlers, params := r.Find("GET", "/users/1")
	assert.Equal(t, 1, len(handlers))
	if assert.Equal(t, 1, len(params)) {
		assert.Equal(t, "1", params["id"])
	}
}

func TestRouterNormalizeRequestPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/", "/"},
		{"/users", "/users"},
		{"/users/", "/users"},
		{"/users//", "/users"},
		{"///", "/"},
	}
	r := New(context.Background())
	r.IgnoreTrailingSlash = true
	for _, test := range tests {
		result := r.normalizeRequestPath(test.path)
		assert.Equal(t, test.expected, result)
	}
}

func TestTimeout(t *testing.T) {
	r := New(context.Background())
	h1 := func(c *Context) error {
		time.Sleep(2 * time.Second)
		return nil
	}
	r.Timeout(1 * time.Second)
	r.add("GET", "/", []Handler{h1})
	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, http.StatusRequestTimeout, res.Code)
	assert.Equal(t, "Request Timeout\n", res.Body.String())
}

func TestCustomTimeout(t *testing.T) {
	r := New(context.Background())
	h1 := func(c *Context) error {
		time.Sleep(2 * time.Second)
		return nil
	}
	r.Timeout(1*time.Second, func(c *Context) error {
		return NewHTTPError(http.StatusRequestTimeout, "Custom Request Timeout")
	})
	r.add("GET", "/", []Handler{h1})
	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, http.StatusRequestTimeout, res.Code)
	assert.Equal(t, "Custom Request Timeout\n", res.Body.String())
}

func TestCombinedTimeout(t *testing.T) {
	fmt.Println("TestCombinedTimeout")
	r := New(context.Background())
	h1 := func(c *Context) error {
		time.Sleep(2 * time.Second)
		select {
		case <-c.Context.Done():
			fmt.Println(c.Context.Err())
			switch c.Context.Err() {
			case context.DeadlineExceeded:
				return NewHTTPError(http.StatusRequestTimeout)
			case context.Canceled:
				return nil
			}
		default:
			c.Write("handler1 Done!")
		}

		return nil
	}
	h2 := func(c *Context) error {
		time.Sleep(2 * time.Second)
		select {
		case <-c.Context.Done():
			switch c.Context.Err() {
			case context.DeadlineExceeded:
				return NewHTTPError(http.StatusRequestTimeout)
			case context.Canceled:
				return nil
			}
		default:
			c.Write("handler2 Done!")
		}
		return nil
	}
	r.Timeout(3 * time.Second)
	r.add("GET", "/", []Handler{h1, h2})
	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, http.StatusRequestTimeout, res.Code)
	assert.Equal(t, "handler1 Done!Request Timeout\n", res.Body.String())
}

func TestNoTimeout(t *testing.T) {
	r := New(context.Background())
	h1 := func(c *Context) error {
		time.Sleep(1 * time.Second)
		c.Write("handler Done!")
		return nil
	}
	r.Timeout(2 * time.Second)
	r.add("GET", "/", []Handler{h1})
	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, "handler Done!", res.Body.String())
}

func TestRouterHandleError(t *testing.T) {
	r := New(context.Background())
	res := httptest.NewRecorder()
	c := &Context{ResponseWriter: res}
	r.handleError(c, errors.New("abc"))
	assert.Equal(t, http.StatusInternalServerError, res.Code)

	res = httptest.NewRecorder()
	c = &Context{ResponseWriter: res}
	r.handleError(c, NewHTTPError(http.StatusNotFound))
	assert.Equal(t, http.StatusNotFound, res.Code)
}

func TestHTTPHandler(t *testing.T) {
	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users/", nil)
	c := NewContext(res, req)

	h1 := HTTPHandlerFunc(http.NotFound)
	err := h1(c)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, res.Code)

	res = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/users/", nil)
	c = NewContext(res, req)
	h2 := HTTPHandler(http.NotFoundHandler())
	err = h2(c)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, res.Code)
}
