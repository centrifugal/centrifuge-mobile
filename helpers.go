package centrifuge

import gocentrifuge "github.com/centrifugal/centrifuge-go"

// Exp ...
func Exp(ttlSeconds int) string {
	return gocentrifuge.Exp(ttlSeconds)
}

// GenerateClientSign ...
func GenerateClientSign(secret, user, exp, info string) string {
	return gocentrifuge.GenerateClientSign(secret, user, exp, info)
}

// GenerateChannelSign ...
func GenerateChannelSign(secret, client, channel, channelData string) string {
	return gocentrifuge.GenerateChannelSign(secret, client, channel, channelData)
}
