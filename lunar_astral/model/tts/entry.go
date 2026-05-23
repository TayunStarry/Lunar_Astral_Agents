package tts

func (e *ttsCacheEntry) MarkReady(audio string) {
	e.audio = audio
	close(e.ready)
}

func (e *ttsCacheEntry) Wait() string {
	<-e.ready
	return e.audio
}
