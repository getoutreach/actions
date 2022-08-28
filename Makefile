APP := actions
OSS := false
_ := $(shell ./scripts/devbase.sh) 

include .bootstrap/root/Makefile

###Block(targets)
.PHONY: new-action
new-action:
	./scripts/new-action.sh $(name)

.PHONY: test-action
test-action:
	./scripts/test-action.sh $(name) $(payload)
###EndBlock(targets)
