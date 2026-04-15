package mqtt

import (
	"strings"
)

// Realm is the default MQTT realm
var Realm = "realm"

// TopicTokens define the indices of parts in a topic hierarchy
var TopicTokens = struct {
	Realm        int
	Type         int
	Namespace    int
	SceneName    int
	SceneMsgType int
	UserClient   int
	UUID         int
	ToUID        int
}{
	Realm:        0,
	Type:         1,
	Namespace:    2,
	SceneName:    3,
	SceneMsgType: 4,
	UserClient:   5,
	UUID:         6,
	ToUID:        7,
}

var SceneMsgTypes = struct {
	Presence string
	Chat     string
	User     string
	Objects  string
	Render   string
	Env      string
	Program  string
	Debug    string
}{
	Presence: "x",
	Chat:     "c",
	User:     "u",
	Objects:  "o",
	Render:   "r",
	Env:      "e",
	Program:  "p",
	Debug:    "d",
}

// Topics contains the literal substitution graph mapping for subscriptions and publishing, mirroring arena-persist
var Topics = struct {
	Subscribe struct {
		Network            string
		Device             string
		RTRuntime          string
		RTModules          string
		ScenePublic        string
		ScenePrivate       string
		SceneRenderPublic  string
		SceneRenderPrivate string
	}
	Publish struct {
		NetworkLatency       string
		Device               string
		RTRuntime            string
		RTModules            string
		ProcDbg              string
		ScenePresence        string
		ScenePresencePrivate string
		SceneChat            string
		SceneChatPrivate     string
		SceneUser            string
		SceneUserPrivate     string
		SceneObjects         string
		SceneObjectsPrivate  string
		SceneRender          string
		SceneRenderPrivate   string
		SceneEnv             string
		SceneEnvPrivate      string
		SceneProgram         string
		SceneProgramPrivate  string
		SceneDebug           string
	}
}{
	Subscribe: struct {
		Network            string
		Device             string
		RTRuntime          string
		RTModules          string
		ScenePublic        string
		ScenePrivate       string
		SceneRenderPublic  string
		SceneRenderPrivate string
	}{
		Network:            "$NETWORK",
		Device:             "{realm}/d/{nameSpace}/{deviceName}/#",
		RTRuntime:          "{realm}/g/{nameSpace}/p/{rtUuid}",
		RTModules:          "{realm}/s/{nameSpace}/{sceneName}/p/+/+",
		ScenePublic:        "{realm}/s/{nameSpace}/{sceneName}/+/+/+",
		ScenePrivate:       "{realm}/s/{nameSpace}/{sceneName}/+/+/+/{idTag}/#",
		SceneRenderPublic:  "{realm}/s/{nameSpace}/{sceneName}/r/+/-",
		SceneRenderPrivate: "{realm}/s/{nameSpace}/{sceneName}/r/+/-/{idTag}/#",
	},
	Publish: struct {
		NetworkLatency       string
		Device               string
		RTRuntime            string
		RTModules            string
		ProcDbg              string
		ScenePresence        string
		ScenePresencePrivate string
		SceneChat            string
		SceneChatPrivate     string
		SceneUser            string
		SceneUserPrivate     string
		SceneObjects         string
		SceneObjectsPrivate  string
		SceneRender          string
		SceneRenderPrivate   string
		SceneEnv             string
		SceneEnvPrivate      string
		SceneProgram         string
		SceneProgramPrivate  string
		SceneDebug           string
	}{
		NetworkLatency:       "$NETWORK/latency",
		Device:               "{realm}/d/{nameSpace}/{deviceName}/{idTag}",
		RTRuntime:            "{realm}/g/{nameSpace}/p/{rtUuid}",
		RTModules:            "{realm}/s/{nameSpace}/{sceneName}/p/{userClient}/{idTag}",
		ProcDbg:              "{realm}/proc/debug/{uuid}",
		ScenePresence:        "{realm}/s/{nameSpace}/{sceneName}/x/{userClient}/{idTag}",
		ScenePresencePrivate: "{realm}/s/{nameSpace}/{sceneName}/x/{userClient}/{idTag}/{toUid}",
		SceneChat:            "{realm}/s/{nameSpace}/{sceneName}/c/{userClient}/{idTag}",
		SceneChatPrivate:     "{realm}/s/{nameSpace}/{sceneName}/c/{userClient}/{idTag}/{toUid}",
		SceneUser:            "{realm}/s/{nameSpace}/{sceneName}/u/{userClient}/{userObj}",
		SceneUserPrivate:     "{realm}/s/{nameSpace}/{sceneName}/u/{userClient}/{userObj}/{toUid}",
		SceneObjects:         "{realm}/s/{nameSpace}/{sceneName}/o/{userClient}/{objectId}",
		SceneObjectsPrivate:  "{realm}/s/{nameSpace}/{sceneName}/o/{userClient}/{objectId}/{toUid}",
		SceneRender:          "{realm}/s/{nameSpace}/{sceneName}/r/{userClient}/{idTag}",
		SceneRenderPrivate:   "{realm}/s/{nameSpace}/{sceneName}/r/{userClient}/{idTag}/-",
		SceneEnv:             "{realm}/s/{nameSpace}/{sceneName}/e/{userClient}/{idTag}",
		SceneEnvPrivate:      "{realm}/s/{nameSpace}/{sceneName}/e/{userClient}/{idTag}/-",
		SceneProgram:         "{realm}/s/{nameSpace}/{sceneName}/p/{userClient}/{idTag}",
		SceneProgramPrivate:  "{realm}/s/{nameSpace}/{sceneName}/p/{userClient}/{idTag}/{toUid}",
		SceneDebug:           "{realm}/s/{nameSpace}/{sceneName}/d/{userClient}/{idTag}/-",
	},
}

// FormatTopic helper replaces string literals similarly to template literals in JS.
// A defensive copy of args is made to avoid mutating the caller's map.
func FormatTopic(topic string, args map[string]string) string {
	merged := make(map[string]string, len(args)+1)
	for k, v := range args {
		merged[k] = v
	}
	merged["realm"] = Realm
	for k, v := range merged {
		topic = strings.ReplaceAll(topic, "{"+k+"}", v)
	}
	return topic
}
