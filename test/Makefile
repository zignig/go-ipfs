# Run tests
#
# Copyright (c) 2014 Christian Couder
# MIT Licensed; see the LICENSE file in this repository.
#

T = $(sort $(wildcard t[0-9][0-9][0-9][0-9]-*.sh))

all: clean $(T) aggregate

clean:
	@echo "*** $@ ***"
	-rm -r test-results

$(T):
	@echo "*** $@ ***"
	./$@

aggregate:
	@echo "*** $@ ***"
	./test-aggregate-results.sh

.PHONY: all clean $(T) aggregate