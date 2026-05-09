```LunarTick
@log "=== LunarTick Test Program Start ==="

SET message "Hello World"
@log "#message"

SET count "0"
@lazy *testLoop
@math count #count + 1
@log "Current count: #count"
@sleep 500

@lazy *end
@log "Test complete!"

*testLoop
```
