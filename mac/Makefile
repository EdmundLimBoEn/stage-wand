SIGN_IDENTITY ?= Apple Development
APP_DIR ?= $(HOME)/Applications/StageWandMac.app

app:
	swift build -c release
	mkdir -p "$(APP_DIR)"/Contents/MacOS
	cp .build/release/StageWandMac "$(APP_DIR)"/Contents/MacOS/StageWandMac.new
	mv "$(APP_DIR)"/Contents/MacOS/StageWandMac.new "$(APP_DIR)"/Contents/MacOS/StageWandMac
	/usr/libexec/PlistBuddy -c 'Clear dict' -c 'Add :CFBundleIdentifier string systems.edmundlim.stagewand.mac' -c 'Add :CFBundleName string StageWandMac' -c 'Add :CFBundleExecutable string StageWandMac' -c 'Add :CFBundlePackageType string APPL' -c 'Add :LSUIElement bool true' -c 'Add :NSLocalNetworkUsageDescription string Connect to Stage Wand on your phone' -c 'Add :NSBonjourServices array' -c 'Add :NSBonjourServices:0 string _stagewand._tcp' "$(APP_DIR)"/Contents/Info.plist
	xattr -cr "$(APP_DIR)"
	codesign --force --sign "$(SIGN_IDENTITY)" "$(APP_DIR)"
