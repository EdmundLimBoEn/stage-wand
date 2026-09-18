app:
	swift build -c release
	mkdir -p StageWandMac.app/Contents/MacOS
	cp .build/release/StageWandMac StageWandMac.app/Contents/MacOS/StageWandMac
	/usr/libexec/PlistBuddy -c 'Clear dict' -c 'Add :CFBundleIdentifier string systems.edmundlim.stagewand.mac' -c 'Add :CFBundleName string StageWandMac' -c 'Add :CFBundleExecutable string StageWandMac' -c 'Add :CFBundlePackageType string APPL' -c 'Add :LSUIElement bool true' -c 'Add :NSLocalNetworkUsageDescription string Connect to Stage Wand on your phone' -c 'Add :NSBonjourServices array' -c 'Add :NSBonjourServices:0 string _stagewand._tcp' StageWandMac.app/Contents/Info.plist
	xattr -cr StageWandMac.app
	codesign --force --sign - StageWandMac.app
