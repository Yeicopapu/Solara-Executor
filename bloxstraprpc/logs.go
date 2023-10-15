package bloxstraprpc

import (
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/hugolgst/rich-go/client"
)

const (
	GameJoiningEntry               = "[FLog::Output] ! Joining game"
	GameJoiningPrivateServerEntry  = "[FLog::GameJoinUtil] GameJoinUtil::joinGamePostPrivateServer"
	GameJoiningReservedServerEntry = "[FLog::GameJoinUtil] GameJoinUtil::initiateTeleportToReservedServer"
	GameJoiningUDMUXEntry          = "[FLog::Network] UDMUX Address = "
	GameJoinedEntry                = "[FLog::Network] serverId:"
	GameDisconnectedEntry          = "[FLog::Network] Time to disconnect replication data:"
	GameTeleportingEntry           = "[FLog::SingleSurfaceApp] initiateTeleport"
	GameMessageEntry               = "[FLog::Output] [BloxstrapRPC]"
)

var (
	GameJoiningEntryPattern = regexp.MustCompile(`! Joining game '([0-9a-f\-]{36})' place ([0-9]+) at ([0-9\.]+)`)
	GameJoiningUDMUXPattern = regexp.MustCompile(`UDMUX Address = ([0-9\.]+), Port = [0-9]+ \| RCC Server Address = ([0-9\.]+), Port = [0-9]+`)
	GameJoinedEntryPattern  = regexp.MustCompile(`serverId: ([0-9\.]+)\|[0-9]+`)
)

type ServerType int

const (
	Public ServerType = iota
	Private
	Reserved
)

type Activity struct {
	presence            client.Activity
	timeStartedUniverse time.Time
	currentUniverseID   string

	InGame     bool
	IsTeleport bool
	ServerType
	PlaceID string
	JobID   string
	MAC     string

	teleport         bool
	reservedteleport bool
}

func (a *Activity) HandleLog(line string) error {
	if !a.InGame && a.PlaceID == "" {
		if strings.Contains(line, GameJoiningPrivateServerEntry) {
			a.ServerType = Private
			return nil
		}

		if strings.Contains(line, GameJoiningEntry) {
			a.handleGameJoining(line)
			return nil
		}
	}

	if !a.InGame && a.PlaceID != "" {
		if strings.Contains(line, GameJoiningUDMUXEntry) {
			a.handleUDMUX(line)
			return nil
		}

		if strings.Contains(line, GameJoinedEntry) {
			a.handleGameJoined(line)

			return a.SetCurrentGame()
		}
	}

	if a.InGame && a.PlaceID != "" {
		if strings.Contains(line, GameDisconnectedEntry) {
			log.Printf("Disconnected From Game (%s/%s/%s)", a.PlaceID, a.JobID, a.MAC)
			a.Clear()
			return a.SetCurrentGame()
		}

		if strings.Contains(line, GameTeleportingEntry) {
			log.Printf("Teleporting to server (%s/%s/%s)", a.PlaceID, a.JobID, a.MAC)
			a.teleport = true
			return nil
		}

		if a.teleport && strings.Contains(line, GameJoiningReservedServerEntry) {
			log.Printf("Teleporting to reserved server")
			a.reservedteleport = true
			return nil
		}

		if strings.Contains(line, GameMessageEntry) {
			m, err := ParseMessage(line)
			if err != nil {
				return err
			}

			a.ProcessMessage(&m)
			return a.UpdatePresence()
		}
	}

	return nil
}

func (a *Activity) handleUDMUX(line string) {
	m := GameJoiningUDMUXPattern.FindStringSubmatch(line)
	if len(m) != 3 || m[2] != a.MAC {
		return
	}

	a.MAC = m[1]
	log.Printf("Got game join UDMUX: %s", a.MAC)
}

func (a *Activity) handleGameJoining(line string) {
	m := GameJoiningEntryPattern.FindStringSubmatch(line)
	if len(m) != 4 {
		return
	}

	a.InGame = false
	a.JobID = m[1]
	a.PlaceID = m[2]
	a.MAC = m[3]

	if a.teleport {
		a.IsTeleport = true
		a.teleport = false
	}

	if a.reservedteleport {
		a.ServerType = Reserved
		a.reservedteleport = false
	}

	log.Printf("Joining Game (%s/%s/%s)", a.JobID, a.PlaceID, a.MAC)
}

func (a *Activity) handleGameJoined(line string) {
	m := GameJoinedEntryPattern.FindStringSubmatch(line)
	if len(m) != 2 || m[1] != a.MAC {
		return
	}

	a.InGame = true
	log.Printf("Joined Game (%s/%s/%s)", a.PlaceID, a.JobID, a.MAC)
	// handle rpc
}

func (a *Activity) Clear() {
	a.InGame = false
	a.PlaceID = ""
	a.JobID = ""
	a.MAC = ""
	a.ServerType = Public
	a.IsTeleport = false
	a.presence = client.Activity{}
}