SHELL := /bin/bash

.PHONY: tag

## tag — создать git-тег: берёт последний тэг, инкрементирует последний
## числовой сегмент (v1.1.1 -> v1.1.2) и даёт отредактировать имя перед созданием.
tag:
	@set -euo pipefail; \
	prev="$$(git describe --tags --abbrev=0 2>/dev/null || true)"; \
	if [ -z "$$prev" ]; then prev="v0.0.0"; fi; \
	base="$${prev#v}"; \
	IFS="." read -ra parts <<< "$$base"; \
	last_idx=$$(( $${#parts[@]} - 1 )); \
	parts[$$last_idx]=$$(( $${parts[$$last_idx]} + 1 )); \
	new="v"; \
	for p in "$${parts[@]}"; do new="$$new$$p."; done; \
	new="$${new%.}"; \
	read -e -p "Предыдущий тэг $$prev. Создаем новый: " -i "$$new" name; \
	if [ -z "$$name" ]; then echo "Имя тэга пустое, операция отменена."; exit 1; fi; \
	git tag "$$name"; \
	git push origin "$$name"; \
	echo "Тэг $$name создан и запушен."