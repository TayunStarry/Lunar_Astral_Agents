package tts

func (r *responseCapture) Write(b []byte) (int, error) {
	r.ResponseWriter.Write(b)
	return r.body.Write(b)
}

func (r *responseCapture) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
