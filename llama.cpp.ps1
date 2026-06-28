./local_data/models/llama.cpp/llama-server.exe `
		"--models-preset", "local_data/models/models.ini" `
		"--flash-attn", "on" `
		"--temp", "1.0" `
		"--top-p", "0.95" `
		"--presence-penalty", "0.0" `
		"--top-k", "20" `
		"--min-p", "0.0" `
		"--repeat_penalty", "1.0" `
		"--port", "12345" `
		"--parallel", "1" `
		"--batch-size", "2048" `
		"--ubatch-size", "1024" `
		"--threads", "8" `
		"--cache-type-k", "q8_0" `
		"--cache-type-v", "q8_0" `
		"--sleep-idle-seconds", "900"
