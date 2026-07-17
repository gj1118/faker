package constants

import "strings"

var TrackerDomains = []string{
	"doubleclick.net", "google-analytics.com", "facebook.com", "ads.twitter.com",
	"scorecardresearch.com", "quantserve.com", "adnxs.com", "rubiconproject.com",
	"pubmatic.com", "openx.net", "adsrvr.org", "casalemedia.com",
	"advertising.com", "amazon-adsystem.com", "criteo.com", "bing.com",
	"taboola.com", "outbrain.com", "spotxchange.com", "sharethrough.com",
	"moatads.com", "chartbeat.com", "newrelic.com", "mixpanel.com",
	"segment.io", "hotjar.com", "optimizely.com", "mopub.com",
	"rlcdn.com", "demdex.net", "bluekai.com", "krxd.net",
}

var CookieNames = []string{
	"_ga", "_gid", "_fbp", "_gcl_au", "IDE", "ANID", "NID",
	"SID", "SSID", "APISID", "SAPISID", "uid", "uuid", "visitor_id",
	"tracking_id", "sess_id", "ad_id", "cid", "c_user", "xs",
	"fr", "datr", "spin", "wd", "act", "presence",
}

var TempFilePatterns = []string{
	"tmp_track_%d.dat", "cache_%d.bin", "sess_%d.tmp", "ad_cache_%d.tmp",
	"pixel_%d.gif.tmp", "beacon_%d.dat", "sync_%d.tmp", "uid_%d.dat",
}

var HistoryURLs = []string{
	"https://www.googleadservices.com/pagead/aclk?sa=L&ai=",
	"https://pixel.facebook.com/tr?id=",
	"https://sync.criteo.com/sync?p=",
	"https://cm.g.doubleclick.net/pixel?google_nid=",
	"https://ib.adnxs.com/getuid?",
	"https://sync.rubiconproject.com/usync?p=",
	"https://x.bidswitch.net/sync?ssp=",
	"https://match.adsrvr.org/track/cmf/generic?ttd_pid=",
	"https://ups.analytics.yahoo.com/ups/",
	"https://s.amazon-adsystem.com/iu3?pid=",
}

var TrashExts = []string{".txt", ".log", ".tmp", ".bak", ".doc", ".csv"}

var LoremWords = strings.Fields(
	"lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt" +
		" ut labore et dolore magna aliqua ut enim ad minim veniam quis nostrud exercitation ullamco" +
		" laboris nisi ut aliquip ex ea commodo consequat duis aute irure dolor in reprehenderit in" +
		" voluptate velit esse cillum dolore eu fugiat nulla pariatur excepteur sint occaecat cupidatat" +
		" non proident sunt in culpa qui officia deserunt mollit anim id est laborum")

var EICAR_Url string = "https://secure.eicar.org/eicar_com.zip"
var EICAR_FILE_NAME = "eicar.com"

const FakerDir = "fake_tracker_test"

// shredderFileSizes defines the pool of file sizes used when generating shredder temp files.
// Files are spread across small (1 KB), medium (64 KB–512 KB), and large (1 MB–10 MB) tiers
// so that the shredder has a realistic variety of workloads to process.
var ShredderFileSizes = []int{
	1 * 1024,         // 1 KB
	4 * 1024,         // 4 KB
	16 * 1024,        // 16 KB
	64 * 1024,        // 64 KB
	256 * 1024,       // 256 KB
	512 * 1024,       // 512 KB
	1 * 1024 * 1024,  // 1 MB
	4 * 1024 * 1024,  // 4 MB
	10 * 1024 * 1024, // 10 MB
}

 const LOG_FILE_NAME = "faker.log"

 const Vol_Name = "FakeVolume"

 const MAX_CPU_USAGE_WARNING = 90
