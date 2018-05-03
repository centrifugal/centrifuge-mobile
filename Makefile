all: release

release:
	@read -p "Enter new release version: " version; \
	./misc/release.sh $$version
