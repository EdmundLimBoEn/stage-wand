SIGN_IDENTITY ?= Apple Development
APP_DIR ?= $(HOME)/Applications/StageWandMac.app

app:
	swift build -c release
	mkdir -p "$(APP_DIR)"/Contents/MacOS
	cp .build/release/StageWandMac "$(APP_DIR)"/Contents/MacOS/StageWandMac.new
	mv "$(APP_DIR)"/Contents/MacOS/StageWandMac.new "$(APP_DIR)"/Contents/MacOS/StageWandMac
	cp Info.plist "$(APP_DIR)"/Contents/Info.plist
	xattr -cr "$(APP_DIR)"
	codesign --force --options runtime --entitlements StageWandMac.entitlements --sign "$(SIGN_IDENTITY)" "$(APP_DIR)"
