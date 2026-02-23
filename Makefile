.PHONY: help build install install-skill test clean

AGENT ?= claude

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build vlt binary
	go build -o vlt ./cmd/vlt

install: ## Install vlt to $GOPATH/bin
	go install ./cmd/vlt

install-skill: ## Install vlt skill for an AI agent (AGENT=claude|codex|opencode)
	@case "$(AGENT)" in \
		claude)   dest="$(HOME)/.claude/skills/vlt-skill" ;; \
		codex)    dest="$(HOME)/.codex/skills/vlt-skill" ;; \
		opencode) dest="$(HOME)/.config/opencode/skills/vlt-skill" ;; \
		*)        echo "Unknown agent: $(AGENT). Use claude, codex, or opencode." >&2; exit 1 ;; \
	esac; \
	mkdir -p "$$dest"; \
	rm -rf "$$dest"; \
	cp -r docs/vlt-skill "$$dest"; \
	echo "Installed vlt skill to $$dest"

test: ## Run tests
	go test -v ./...

clean: ## Remove build artifacts
	rm -f vlt
