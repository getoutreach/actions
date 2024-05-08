APP := actions
OSS := true
_ := $(shell ./scripts/devbase.sh) 

include .bootstrap/root/Makefile

## <<Stencil::Block(targets)>>
.PHONY: new-action
new-action:
	./scripts/new-action.sh $(name)

.PHONY: test-action
test-action:
	./scripts/test-action.sh $(name) $(payload)

post-stencil::
	./scripts/shell-wrapper.sh catalog-sync.sh
## <</Stencil::Block>>
