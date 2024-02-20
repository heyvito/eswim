package resources

import _ "embed"

//go:embed head.html
var StatePageHead string

//go:embed server.html
var StatePageServer string

//go:embed footer.html
var StatePageFooter string

//go:embed script.js
var StatePageScript string
