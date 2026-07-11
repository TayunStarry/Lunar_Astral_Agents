package napcat

// napcat 包全局变量

import (
	"net/http"
	"time"
)

var HTTPClient = &http.Client{Timeout: 10 * time.Second}
