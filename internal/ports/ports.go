package ports

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type PortKey string

const (
	DefaultOffset = 100

	KeyAPIPort           PortKey = "api.port"
	KeyDBPort            PortKey = "db.port"
	KeyDBShadowPort      PortKey = "db.shadow_port"
	KeyDBPoolerPort      PortKey = "db.pooler.port"
	KeyStudioPort        PortKey = "studio.port"
	KeyInbucketPort      PortKey = "inbucket.port"
	KeyInbucketSMTPPort  PortKey = "inbucket.smtp_port"
	KeyInbucketPOP3Port  PortKey = "inbucket.pop3_port"
	KeyAnalyticsPort     PortKey = "analytics.port"
	KeyEdgeInspectorPort PortKey = "edge_runtime.inspector_port"
)

type Settings struct {
	Offset int `yaml:"offset"`
}

type PortMap map[PortKey]int

var defaultBases = map[PortKey]int{
	KeyAPIPort:           54321,
	KeyDBPort:            54322,
	KeyDBShadowPort:      54320,
	KeyDBPoolerPort:      54329,
	KeyStudioPort:        54323,
	KeyInbucketPort:      54324,
	KeyInbucketSMTPPort:  54325,
	KeyInbucketPOP3Port:  54326,
	KeyAnalyticsPort:     54327,
	KeyEdgeInspectorPort: 8083,
}

var managedKeys = []PortKey{
	KeyAPIPort,
	KeyDBPort,
	KeyDBShadowPort,
	KeyDBPoolerPort,
	KeyStudioPort,
	KeyInbucketPort,
	KeyInbucketSMTPPort,
	KeyInbucketPOP3Port,
	KeyAnalyticsPort,
	KeyEdgeInspectorPort,
}

var displayKeys = []PortKey{
	KeyDBShadowPort,
	KeyAPIPort,
	KeyDBPort,
	KeyStudioPort,
	KeyInbucketPort,
	KeyAnalyticsPort,
	KeyDBPoolerPort,
}

var availabilityChecker = func(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func LoadSettings(path string) (Settings, error) {
	settings := Settings{Offset: DefaultOffset}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return Settings{}, fmt.Errorf("read lazybase settings: %w", err)
	}

	if err := yaml.Unmarshal(raw, &settings); err != nil {
		return Settings{}, fmt.Errorf("parse lazybase settings: %w", err)
	}

	return normalizeSettings(settings), nil
}

func DefaultBasePorts() map[PortKey]int {
	copyMap := make(map[PortKey]int, len(defaultBases))
	for key, value := range defaultBases {
		copyMap[key] = value
	}
	return copyMap
}

func ManagedKeys() []PortKey {
	keys := make([]PortKey, len(managedKeys))
	copy(keys, managedKeys)
	return keys
}

func Compute(settings Settings, slot int, keys []PortKey) PortMap {
	settings = normalizeSettings(settings)
	ports := make(PortMap, len(keys))
	for _, key := range keys {
		base, ok := defaultBases[key]
		if !ok {
			continue
		}
		ports[key] = base + slot*settings.Offset
	}
	return ports
}

func normalizeSettings(settings Settings) Settings {
	if settings.Offset <= 0 {
		settings.Offset = DefaultOffset
	}
	return settings
}

func AllAvailable(portMap PortMap) bool {
	for _, port := range portMap {
		if !availabilityChecker(port) {
			return false
		}
	}
	return true
}

func StudioURL(portMap PortMap) string {
	port, ok := portMap[KeyStudioPort]
	if !ok {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func RangeSummary(portMap PortMap) string {
	if len(portMap) == 0 {
		return "-"
	}

	numbers := make([]int, 0, len(portMap))
	seen := make(map[int]struct{}, len(portMap))
	for _, port := range portMap {
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		numbers = append(numbers, port)
	}

	sort.Ints(numbers)
	if len(numbers) == 1 {
		return fmt.Sprintf("%d", numbers[0])
	}
	return fmt.Sprintf("%d-%d", numbers[0], numbers[len(numbers)-1])
}

func DisplayKeys() []PortKey {
	keys := make([]PortKey, len(displayKeys))
	copy(keys, displayKeys)
	return keys
}

func SetAvailabilityCheckerForTests(checker func(port int) bool) func() {
	previous := availabilityChecker
	availabilityChecker = checker
	return func() {
		availabilityChecker = previous
	}
}

func SortKeys(keys []PortKey) []PortKey {
	sorted := make([]PortKey, len(keys))
	copy(sorted, keys)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.Compare(string(sorted[i]), string(sorted[j])) < 0
	})
	return sorted
}
