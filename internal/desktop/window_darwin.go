// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build cgo

package desktop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static NSMenuItem *item(NSString *title, SEL action, NSString *key) {
	return [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:key];
}

// The web view's UI delegate is a weak reference, this one is held for
// the life of the process. It shows the file chooser of an input and
// the page's prompt.
@interface GosintUIDelegate : NSObject <WKUIDelegate>
@end

@implementation GosintUIDelegate

- (void)webView:(WKWebView *)webView
    runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters
              initiatedByFrame:(WKFrameInfo *)frame
             completionHandler:(void (^)(NSArray<NSURL *> *))completionHandler {
	NSOpenPanel *panel = [NSOpenPanel openPanel];
	panel.canChooseFiles = YES;
	panel.canChooseDirectories = parameters.allowsDirectories;
	panel.allowsMultipleSelection = parameters.allowsMultipleSelection;
	[panel beginSheetModalForWindow:webView.window
	              completionHandler:^(NSModalResponse r) {
		completionHandler(r == NSModalResponseOK ? panel.URLs : nil);
	}];
}

- (void)webView:(WKWebView *)webView
    runJavaScriptTextInputPanelWithPrompt:(NSString *)prompt
                              defaultText:(NSString *)defaultText
                         initiatedByFrame:(WKFrameInfo *)frame
                        completionHandler:(void (^)(NSString *))completionHandler {
	NSAlert *alert = [[NSAlert alloc] init];
	alert.messageText = prompt;
	[alert addButtonWithTitle:@"OK"];
	[alert addButtonWithTitle:@"Cancel"];
	NSTextField *field = [[NSTextField alloc] initWithFrame:NSMakeRect(0, 0, 320, 24)];
	field.stringValue = defaultText ?: @"";
	alert.accessoryView = field;
	alert.window.initialFirstResponder = field;
	[alert beginSheetModalForWindow:webView.window
	              completionHandler:^(NSModalResponse r) {
		completionHandler(r == NSAlertFirstButtonReturn ? field.stringValue : nil);
	}];
}

@end

static GosintUIDelegate *uiDelegate;

static void gosintSetUIDelegate(void *win) {
	NSWindow *window = (__bridge NSWindow *)win;
	id view = window.contentView;
	if (![view isKindOfClass:[WKWebView class]]) {
		return;
	}
	uiDelegate = [[GosintUIDelegate alloc] init];
	((WKWebView *)view).UIDelegate = uiDelegate;
}

// The key equivalents of a Mac app live in its menu, without one the
// web view gets no copy, paste or quit.
static void gosintInstallMenu(void) {
	NSMenu *bar = [[NSMenu alloc] init];

	NSMenuItem *appItem = [[NSMenuItem alloc] init];
	NSMenu *app = [[NSMenu alloc] initWithTitle:@"gosint"];
	[app addItem:item(@"Hide gosint", @selector(hide:), @"h")];
	[app addItem:[NSMenuItem separatorItem]];
	[app addItem:item(@"Quit gosint", @selector(terminate:), @"q")];
	[appItem setSubmenu:app];
	[bar addItem:appItem];

	NSMenuItem *editItem = [[NSMenuItem alloc] init];
	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
	[edit addItem:item(@"Undo", @selector(undo:), @"z")];
	NSMenuItem *redo = item(@"Redo", @selector(redo:), @"z");
	[redo setKeyEquivalentModifierMask:NSEventModifierFlagCommand | NSEventModifierFlagShift];
	[edit addItem:redo];
	[edit addItem:[NSMenuItem separatorItem]];
	[edit addItem:item(@"Cut", @selector(cut:), @"x")];
	[edit addItem:item(@"Copy", @selector(copy:), @"c")];
	[edit addItem:item(@"Paste", @selector(paste:), @"v")];
	[edit addItem:item(@"Select All", @selector(selectAll:), @"a")];
	[editItem setSubmenu:edit];
	[bar addItem:editItem];

	NSMenuItem *windowItem = [[NSMenuItem alloc] init];
	NSMenu *window = [[NSMenu alloc] initWithTitle:@"Window"];
	[window addItem:item(@"Minimize", @selector(performMiniaturize:), @"m")];
	[window addItem:item(@"Close", @selector(performClose:), @"w")];
	[windowItem setSubmenu:window];
	[bar addItem:windowItem];

	[[NSApplication sharedApplication] setMainMenu:bar];
	[[NSApplication sharedApplication] setWindowsMenu:window];
}
*/
import "C"

import "unsafe"

// prepareWindow sets the application menu and a UI delegate that
// stays. Called on the main thread once the window exists.
func prepareWindow(win unsafe.Pointer) {
	C.gosintInstallMenu()
	C.gosintSetUIDelegate(win)
}
