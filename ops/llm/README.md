# Local LLM stack

This repo owns the local llama.cpp stack used for indexing and local OpenAI-compatible runtime experiments.

Managed services:
- qwen-coder on http://localhost:12434
- nomic-embed on http://localhost:12435

Files:
- compose file: `ops/llm/docker-compose.yml`
- model directory: `ops/llm/models/`
- logs directory: `ops/llm/logs/`

Model migration from the old `~/LLM` layout:
- old model location: `~/LLM/gguf/`
- new canonical location: `ops/llm/models/`

Accepted migration paths:
- copy GGUF files into `ops/llm/models/`
- symlink individual GGUF files from `ops/llm/models/` to `~/LLM/gguf/...`
- replace `ops/llm/models/` with a symlink to `~/LLM/gguf`

Expected filenames:
- `Qwen2.5-Coder-7B-Instruct-Q6_K_L.gguf`
- `nomic-embed-code.Q8_0.gguf`

Prerequisites:
- Docker with `docker compose`
- NVIDIA GPU runtime available to Docker
- external docker network `llm-net` (the CLI can create it when `local_services.auto_create_networks: true`)

Operator flow:
1. Put or symlink the model files into `ops/llm/models/`.
2. Check readiness with `./bin/yard llm status`.
3. If `local_services.mode: auto`, use `./bin/yard llm up` to bring the stack up.
4. Build the code index with `./bin/yard index`.

Policy modes:
- `off`: never start containers; only report missing services
- `manual`: do not auto-start; return remediation and exact compose paths
- `auto`: create missing networks, run `docker compose up -d`, and wait for health

Useful commands:
- `./bin/yard llm status`
- `./bin/yard llm up`
- `./bin/yard llm down`
- `./bin/yard llm logs`

Common failures:
- Docker daemon down: start Docker and rerun `yard llm status`
- missing NVIDIA runtime: verify Docker can access the GPU and the llama.cpp CUDA image runs correctly
- missing model files: confirm the two expected GGUF filenames exist under `ops/llm/models/`
- missing `llm-net`: rerun `yard llm up` with auto-create enabled or create it manually with `docker network create llm-net`
- health endpoint never ready: inspect `yard llm logs` and confirm the selected ports are free on the host
