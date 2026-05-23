package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/go-faster/errors"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/extension"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// exportFile matches the JSON structure produced by `tdl chat export`
type exportFile struct {
	ID       int64           `json:"id"`
	Messages []exportMessage `json:"messages"`
}

type exportMessage struct {
	ID int `json:"id"`
}

func main() {
	conf := zap.NewDevelopmentConfig()
	conf.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logger := zap.Must(conf.Build(zap.AddCaller()))

	extension.New(extension.Options{
		Logger: logger,
	})(func(ctx context.Context, e *extension.Extension) error {
		return rootCmd(ctx, e).Execute()
	})
}

func rootCmd(ctx context.Context, e *extension.Extension) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete Telegram messages",
		Long: `Delete messages from Telegram.

Supports three input modes (can be combined):
  1. --from <file.json>   read from tdl chat export JSON (can be repeated)
  2. --url <t.me/c/...>   individual message URLs (can be repeated)
  3. --chat <id> --id <n> explicit chat + message IDs`,
	}

	var (
		fromFiles  []string
		msgURLs    []string
		chatFlag   string
		msgIDs     []int
		revokeFlag bool
	)

	cmd.Flags().StringArrayVar(&fromFiles, "from", nil, "tdl chat export JSON file(s) to delete from")
	cmd.Flags().StringArrayVar(&msgURLs, "url", nil, "Message URL(s) to delete (https://t.me/c/CHATID/MSGID)")
	cmd.Flags().StringVar(&chatFlag, "chat", "", "Chat username or numeric ID (with --id)")
	cmd.Flags().IntSliceVar(&msgIDs, "id", nil, "Message ID(s) to delete (with --chat)")
	cmd.Flags().BoolVar(&revokeFlag, "revoke", true, "Revoke for all users")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		api := e.Client().API()

		// chatID -> []msgID
		byChat := map[int64][]int{}

		// -- from JSON export files --
		for _, path := range fromFiles {
			export, err := readExportFile(path)
			if err != nil {
				return errors.Wrapf(err, "read export file %s", path)
			}
			for _, m := range export.Messages {
				byChat[export.ID] = append(byChat[export.ID], m.ID)
			}
			e.Log().Info("loaded export",
				zap.String("file", path),
				zap.Int64("chat_id", export.ID),
				zap.Int("count", len(export.Messages)))
		}

		// -- from message URLs --
		for _, u := range msgURLs {
			chatID, msgID, err := parseMsgURL(u)
			if err != nil {
				return errors.Wrapf(err, "parse URL %s", u)
			}
			byChat[chatID] = append(byChat[chatID], msgID)
		}

		// -- from --chat + --id --
		if chatFlag != "" && len(msgIDs) > 0 {
			peer, err := resolvePeer(ctx, api, chatFlag)
			if err != nil {
				return errors.Wrap(err, "resolve peer")
			}
			switch p := peer.(type) {
			case *tg.InputPeerChannel:
				byChat[p.ChannelID] = append(byChat[p.ChannelID], msgIDs...)
			case *tg.InputPeerChat, *tg.InputPeerUser:
				// regular group / DM — use MessagesDeleteMessages (no channel ID needed)
				affected, err := api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
					Revoke: revokeFlag,
					ID:     msgIDs,
				})
				if err != nil {
					return errors.Wrap(err, "delete messages")
				}
				e.Log().Info("deleted", zap.Int("pts", affected.Pts))
				fmt.Printf("Deleted %d message(s)\n", len(msgIDs))
				return nil
			}
		}

		if len(byChat) == 0 {
			return errors.New("no messages specified; use --from, --url, or --chat + --id")
		}

		// delete per chat
		total := 0
		for chatID, ids := range byChat {
			n, err := deleteChannelMessages(ctx, e, api, chatID, ids, revokeFlag)
			if err != nil {
				return err
			}
			total += n
		}
		fmt.Printf("Total deleted: %d message(s)\n", total)
		return nil
	}

	return cmd
}

// deleteChannelMessages deletes messages from a channel/supergroup in batches of 100.
// Falls back to MessagesDeleteMessages if ChannelsDeleteMessages fails (regular groups).
func deleteChannelMessages(ctx context.Context, e *extension.Extension, api *tg.Client, chatID int64, ids []int, revoke bool) (int, error) {
	inputChannel := &tg.InputChannel{ChannelID: chatID}

	// fetch access hash
	ch, err := api.ChannelsGetChannels(ctx, []tg.InputChannelClass{inputChannel})
	if err == nil {
		if chats := ch.GetChats(); len(chats) > 0 {
			if channel, ok := chats[0].(*tg.Channel); ok {
				inputChannel.AccessHash = channel.AccessHash
			}
		}
	}

	deleted := 0
	// MTProto allows max 100 IDs per call
	for i := 0; i < len(ids); i += 100 {
		end := i + 100
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		affected, err := api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: inputChannel,
			ID:      batch,
		})
		if err != nil {
			// fallback for regular groups (not channels)
			e.Log().Warn("ChannelsDeleteMessages failed, trying MessagesDeleteMessages",
				zap.Int64("chat_id", chatID), zap.Error(err))
			aff2, err2 := api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
				Revoke: revoke,
				ID:     batch,
			})
			if err2 != nil {
				return deleted, errors.Wrapf(err2, "delete messages in chat %d", chatID)
			}
			deleted += len(batch)
			e.Log().Info("deleted (fallback)",
				zap.Int64("chat_id", chatID),
				zap.Int("batch", len(batch)),
				zap.Int("pts", aff2.Pts))
			continue
		}

		deleted += len(batch)
		e.Log().Info("deleted",
			zap.Int64("chat_id", chatID),
			zap.Int("batch", len(batch)),
			zap.Int("pts", affected.Pts))
	}

	fmt.Printf("Deleted %d message(s) from chat %d\n", deleted, chatID)
	return deleted, nil
}

func readExportFile(path string) (*exportFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var export exportFile
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, errors.Wrap(err, "parse JSON")
	}
	if len(export.Messages) == 0 {
		return nil, errors.New("no messages in export file")
	}
	return &export, nil
}

// parseMsgURL parses:
//
//	https://t.me/c/CHATID/MSGID
//	https://t.me/c/CHATID/THREADID/MSGID
func parseMsgURL(u string) (chatID int64, msgID int, err error) {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "t.me/c/")
	parts := strings.Split(strings.Trim(u, "/"), "/")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid message URL: %s", u)
	}
	chatID, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid chat ID in URL: %s", parts[0])
	}
	msgID64, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid message ID in URL: %s", parts[len(parts)-1])
	}
	return chatID, int(msgID64), nil
}

// resolvePeer resolves a chat string to an InputPeer.
// Accepts: me/self (Saved Messages), @username, username, or numeric ID.
func resolvePeer(ctx context.Context, api *tg.Client, chat string) (tg.InputPeerClass, error) {
	chat = strings.TrimPrefix(chat, "@")

	if chat == "me" || chat == "self" {
		return &tg.InputPeerSelf{}, nil
	}

	if id, err := strconv.ParseInt(chat, 10, 64); err == nil {
		if id < -1000000000000 {
			return &tg.InputPeerChannel{ChannelID: -(id + 1000000000000)}, nil
		}
		if id < 0 {
			return &tg.InputPeerChat{ChatID: -id}, nil
		}
		return &tg.InputPeerUser{UserID: id}, nil
	}

	resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: chat,
	})
	if err != nil {
		return nil, errors.Wrap(err, "resolve username")
	}

	switch p := resolved.Peer.(type) {
	case *tg.PeerChannel:
		for _, c := range resolved.Chats {
			if ch, ok := c.(*tg.Channel); ok && ch.ID == p.ChannelID {
				return &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}, nil
			}
		}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: p.ChatID}, nil
	case *tg.PeerUser:
		for _, u := range resolved.Users {
			if user, ok := u.(*tg.User); ok && user.ID == p.UserID {
				return &tg.InputPeerUser{UserID: user.ID, AccessHash: user.AccessHash}, nil
			}
		}
	}

	return nil, errors.New("could not resolve peer from response")
}
