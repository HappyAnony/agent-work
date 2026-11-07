// System-framework-only, short-lived converter. No source paths or credentials.
#import <AppKit/AppKit.h>
#import <WebKit/WebKit.h>
#import <PDFKit/PDFKit.h>

static void fail(NSString *code) {
    [[NSFileHandle fileHandleWithStandardError] writeData:[[code stringByAppendingString:@"\n"] dataUsingEncoding:NSUTF8StringEncoding]];
    exit(2);
}

@interface WikiPDF : NSObject <WKNavigationDelegate>
@property NSWindow *window;
@property WKWebView *web;
@property NSURL *directory;
@property NSPrintOperation *operation;
- (void)start:(NSString *)html;
@end

@implementation WikiPDF
- (void)start:(NSString *)html {
    self.directory = [NSURL fileURLWithPath:NSFileManager.defaultManager.currentDirectoryPath isDirectory:YES];
    NSDictionary *attrs = [NSFileManager.defaultManager attributesOfItemAtPath:self.directory.path error:nil];
    if (!attrs || ([attrs[NSFilePosixPermissions] unsignedIntValue] & 0077) ||
        [NSFileManager.defaultManager fileExistsAtPath:[self.directory URLByAppendingPathComponent:@"output.pdf"].path]) fail(@"pdf_private_directory_required");
    [NSApplication.sharedApplication setActivationPolicy:NSApplicationActivationPolicyProhibited];
    WKWebViewConfiguration *config = [WKWebViewConfiguration new];
    config.websiteDataStore = WKWebsiteDataStore.nonPersistentDataStore;
    config.defaultWebpagePreferences.allowsContentJavaScript = NO;
    config.preferences.javaScriptCanOpenWindowsAutomatically = NO;
    WKContentRuleListStore *store = [WKContentRuleListStore storeWithURL:[self.directory URLByAppendingPathComponent:@"rules" isDirectory:YES]];
    NSString *rules = @"[{\"trigger\":{\"url-filter\":\".*\"},\"action\":{\"type\":\"block\"}},{\"trigger\":{\"url-filter\":\"^data:\"},\"action\":{\"type\":\"ignore-previous-rules\"}},{\"trigger\":{\"url-filter\":\"^about:blank$\"},\"action\":{\"type\":\"ignore-previous-rules\"}}]";
    [store compileContentRuleListForIdentifier:@"wiki-offline" encodedContentRuleList:rules completionHandler:^(WKContentRuleList *list, NSError *error) {
        if (!list || error) fail(@"pdf_resource_policy_failed");
        [config.userContentController addContentRuleList:list];
        self.web = [[WKWebView alloc] initWithFrame:NSMakeRect(0, 0, 523.28, 769.89) configuration:config];
        self.web.navigationDelegate = self;
        self.window = [[NSWindow alloc] initWithContentRect:self.web.frame styleMask:NSWindowStyleMaskBorderless backing:NSBackingStoreBuffered defer:NO];
        self.window.contentView = self.web;
        NSString *printCSS = @"<style>@page{size:A4;margin:36pt}@media print{html,body{margin:0!important;padding:0!important}h1,h2,h3{break-after:avoid}tr,img{break-inside:avoid}}</style>";
        [self.web loadHTMLString:[html stringByAppendingString:printCSS] baseURL:nil];
    }];
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 18 * NSEC_PER_SEC), dispatch_get_main_queue(), ^{ fail(@"pdf_timeout"); });
}
- (void)webView:(WKWebView *)web decidePolicyForNavigationAction:(WKNavigationAction *)action decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
    decisionHandler([action.request.URL.absoluteString isEqualToString:@"about:blank"] ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}
- (void)webView:(WKWebView *)web didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error { fail(@"pdf_load_failed"); }
- (void)webView:(WKWebView *)web didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error { fail(@"pdf_load_failed"); }
- (void)webViewWebContentProcessDidTerminate:(WKWebView *)web { fail(@"pdf_renderer_terminated"); }
- (void)webView:(WKWebView *)web didFinishNavigation:(WKNavigation *)navigation { [self ready]; }
- (void)ready {
    // App-authored inspection only; page scripts are disabled. Wait for actual
    // image decoding and fonts rather than guessing a delay after didFinish.
    [self.web evaluateJavaScript:@"document.fonts.status === 'loaded' && Array.from(document.images).every(i => !i.getAttribute('src') || (i.complete && (!i.src.startsWith('data:image/') || i.naturalWidth > 0)))" completionHandler:^(id value, NSError *error) {
        if (error) fail(@"pdf_readiness_failed");
        if (![value boolValue]) { dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 50 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{[self ready];}); return; }
        [self.web evaluateJavaScript:@"Math.max(document.body.scrollHeight, document.documentElement.scrollHeight)" completionHandler:^(id height, NSError *error) {
            if (error || ![height isKindOfClass:NSNumber.class] || !isfinite([height doubleValue]) || [height doubleValue] > 150000) fail(@"pdf_page_limit");
            [self render];
        }];
    }];
}
- (void)render {
    NSURL *output = [self.directory URLByAppendingPathComponent:@"output.pdf"];
    NSPrintInfo *info = [[NSPrintInfo alloc] initWithDictionary:@{}];
    info.paperSize = NSMakeSize(595.28, 841.89);
    info.topMargin = 36; info.bottomMargin = 36; info.leftMargin = 36; info.rightMargin = 36;
    info.horizontalPagination = NSPrintingPaginationModeFit; info.verticalPagination = NSPrintingPaginationModeAutomatic;
    info.horizontallyCentered = NO; info.verticallyCentered = NO;
    info.jobDisposition = NSPrintSaveJob;
    [info dictionary][NSPrintJobSavingURL] = output;
    self.operation = [self.web printOperationWithPrintInfo:info];
    self.operation.showsPrintPanel = NO; self.operation.showsProgressPanel = NO;
    [self.operation runOperationModalForWindow:self.window delegate:self didRunSelector:@selector(printFinished:success:context:) contextInfo:nil];
}
- (void)printFinished:(NSPrintOperation *)operation success:(BOOL)success context:(void *)context {
    if (!success) fail(@"pdf_print_failed");
    NSURL *output = [self.directory URLByAppendingPathComponent:@"output.pdf"];
    NSNumber *size; [output getResourceValue:&size forKey:NSURLFileSizeKey error:nil];
    if (!size || size.unsignedLongLongValue > 16 * 1024 * 1024) fail(@"pdf_output_invalid");
    NSData *data = [NSData dataWithContentsOfURL:output];
    PDFDocument *document = [[PDFDocument alloc] initWithData:data];
    if (data.length < 50 || !document || document.pageCount == 0 || document.pageCount > 200) fail(@"pdf_output_invalid");
    [[NSFileHandle fileHandleWithStandardOutput] writeData:data];
    exit(0);
}
@end

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        if (argc == 2 && !strcmp(argv[1], "--help")) { puts("wiki-pdf --html-stdin | --inspect-stdin\nmacOS offline WebKit converter; bounded UTF-8 HTML stdin, PDF stdout. Private temporary cwd required. No source paths, scripts, network or print dialogs. Inspect accepts bounded PDF stdin and returns JSON page/text metadata without starting WebKit."); return 0; }
        if (argc == 2 && !strcmp(argv[1], "--version")) { puts("wiki-pdf 1"); return 0; }
        if (argc != 2 || (strcmp(argv[1], "--html-stdin") && strcmp(argv[1], "--inspect-stdin"))) fail(@"invalid_argument");
        NSMutableData *input = [NSMutableData data];
        while (YES) {
            NSData *part = [[NSFileHandle fileHandleWithStandardInput] readDataOfLength:32768];
            if (!part.length) break;
            [input appendData:part]; if (input.length > 16 * 1024 * 1024) fail(@"pdf_input_limit");
        }
        if (!strcmp(argv[1], "--inspect-stdin")) {
            PDFDocument *document = [[PDFDocument alloc] initWithData:input];
            if (!document || document.pageCount == 0 || document.pageCount > 200) fail(@"pdf_input_invalid");
            NSMutableArray *sizes = [NSMutableArray array];
            for (NSUInteger i=0; i<document.pageCount; i++) { CGRect rect = [[document pageAtIndex:i] boundsForBox:kPDFDisplayBoxMediaBox]; [sizes addObject:@[@(rect.size.width),@(rect.size.height)]]; }
            NSData *json = [NSJSONSerialization dataWithJSONObject:@{@"pageCount":@(document.pageCount),@"text":document.string ?: @"",@"pageSizes":sizes} options:0 error:nil];
            [[NSFileHandle fileHandleWithStandardOutput] writeData:json]; return 0;
        }
        NSString *html = [[NSString alloc] initWithData:input encoding:NSUTF8StringEncoding];
        if (!html.length) fail(@"pdf_input_invalid");
        WikiPDF *renderer = [WikiPDF new]; [renderer start:html];
        [NSApplication.sharedApplication run];
    }
    return 2;
}
