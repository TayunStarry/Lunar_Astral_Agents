package tts

func (c *ttsCache) Get(text string) (*ttsCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.items[text]
	if exists {
		for i, t := range c.order {
			if t == text {
				c.order = append(c.order[:i], c.order[i+1:]...)
				c.order = append(c.order, text)
				break
			}
		}
		return entry, true
	}
	return nil, false
}

func (c *ttsCache) GetOrSetPending(text string) (*ttsCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.items[text]; exists {
		return entry, false
	}

	if len(c.order) >= ttsWrapperCacheMax {
		oldest := c.order[0]
		delete(c.items, oldest)
		c.order = c.order[1:]
	}

	entry := &ttsCacheEntry{
		ready: make(chan struct{}),
	}
	c.items[text] = entry
	c.order = append(c.order, text)
	return entry, true
}

func (c *ttsCache) Remove(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, text)
	for i, t := range c.order {
		if t == text {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}
