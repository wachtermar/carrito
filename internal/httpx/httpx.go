package httpx

import "fmt"

type StatusError struct {
	Method     string
	URL        string
	StatusCode int
	Status     string
	Body       string
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s %s: %s", e.Method, e.URL, e.Status)
	}
	return fmt.Sprintf("%s %s: %s: %s", e.Method, e.URL, e.Status, e.Body)
}
