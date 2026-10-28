package util

// bindingRevision identifies the gomobile surface this build publishes. The
// native plugins call BindingRevision at startup and refuse to run against an
// older artifact than they were written for.
//
// Without it the two drift silently: the aar is not in the app's git tree, so
// nothing shows that the plugin and the binary disagree. On Android the first
// sign is a NoSuchMethodException in the middle of a signature; on iOS it is a
// build failure, which is at least loud.
//
// Bump this whenever an exported signature changes or disappears.
//
//	1  pre-session surface (through wallet-sdk v0.0.7)
//	2  signing sessions, EIP-191 and EIP-712 signers, WalletConnect describers,
//	   and SignTypedDataJSON gaining its expectedSigner argument
const bindingRevision = 2

func BindingRevision() int {
	return bindingRevision
}
