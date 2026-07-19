# Build the Seedstream plugin egg.
#
# IMPORTANT: the egg's Python version must match the Python your Deluge
# DAEMON runs (eggs are picked up by exact major.minor). Build with that
# interpreter, e.g. inside the Deluge container:
#   docker exec -w /plugin deluge python3 setup.py bdist_egg
PYTHON ?= python3

.PHONY: egg clean

egg:
	$(PYTHON) setup.py bdist_egg
	@echo "egg built under dist/ — copy it to the Deluge plugins directory"

clean:
	rm -rf build dist *.egg-info
