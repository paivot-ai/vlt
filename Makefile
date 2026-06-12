.PHONY: help build install install-skill test clean bump

SKILL_MD := docs/vlt-skill/SKILL.md

AGENT ?= claude
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

bump: ## Bump committed skill version: make bump v=0.12.0 (run BEFORE tagging a release)
ifndef v
	$(error Usage: make bump v=X.Y.Z)
endif
	@python3 -c "\
import io, re, sys; \
path = '$(SKILL_MD)'; \
text = io.open(path, encoding='utf-8').read(); \
new, n = re.subn(r'(?m)^version:\s*\S+\s*$$', 'version: $(v)', text, count=1); \
sys.exit('ERROR: no version: frontmatter line found in ' + path) if n != 1 else None; \
io.open(path, 'w', encoding='utf-8').write(new); \
print('OK: $(SKILL_MD) -> $(v)')"
	@echo "Skill version synced to $(v). Commit, then tag v$(v)."

build: ## Build vlt binary
	go build -ldflags "$(LDFLAGS)" -o vlt ./cmd/vlt

install: ## Install vlt to $GOPATH/bin
	go install -ldflags "$(LDFLAGS)" ./cmd/vlt

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
