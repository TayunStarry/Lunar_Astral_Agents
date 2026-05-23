package tts

import "net/http"

func (w *ttsMockWriter) Header() http.Header {
	return make(http.Header)
}

func (w *ttsMockWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *ttsMockWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}
