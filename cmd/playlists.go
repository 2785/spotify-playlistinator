package cmd

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/gammazero/workerpool"
	"github.com/gocarina/gocsv"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

// playlistsCmd represents the playlists command
var playlistsCmd = &cobra.Command{
	Use:   "playlists",
	Short: "utils to manage entire playlists",
}

var playlistListConf = struct {
	json string
	csv  string
}{}

var playlistPurgeConf = struct {
	csv    string
	dryrun bool
}{}

func init() {
	listPlaylistCmd.Flags().StringVar(&playlistListConf.json, "json", "", "output playlist list as json")
	listPlaylistCmd.Flags().StringVar(&playlistListConf.csv, "csv", "", "output playlist list as csv")

	purgePlaylistCmd.Flags().StringVar(&playlistPurgeConf.csv, "csv", "", "input csv to purge from")
	purgePlaylistCmd.Flags().BoolVar(&playlistPurgeConf.dryrun, "dryrun", false, "dryrun")

	playlistsCmd.AddCommand(listPlaylistCmd)
	playlistsCmd.AddCommand(purgePlaylistCmd)
	rootCmd.AddCommand(playlistsCmd)
}

type playlistRow struct {
	ID     string `csv:"id"`
	Name   string `csv:"name"`
	Owner  string `csv:"owner"`
	Delete bool   `csv:"delete,omitempty"`
}

var listPlaylistCmd = &cobra.Command{
	Use:   "list",
	Short: "list all playlists",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		zap.L().Info("listing playlists")
		spClient, err := getClient(ctx)
		if err != nil {
			return err
		}

		listRes, err := spClient.CurrentUsersPlaylists(ctx, spotify.Limit(50))
		if err != nil {
			return err
		}

		playlists := listRes.Playlists

		for {
			err = spClient.NextPage(ctx, listRes)
			if err == spotify.ErrNoMorePages {
				break
			}

			if err != nil {
				return err
			}

			playlists = append(playlists, listRes.Playlists...)
		}

		zap.L().Info("found", zap.Int("playlists", len(playlists)))

		if playlistListConf.json != "" {
			bytes, err := json.Marshal(playlists)
			if err != nil {
				return err
			}

			err = os.WriteFile(playlistListConf.json, bytes, os.ModePerm)
			if err != nil {
				return err
			}
		}

		if playlistListConf.csv != "" {
			rows := make([]playlistRow, len(playlists))
			for i, playlist := range playlists {
				rows[i] = playlistRow{
					ID:    string(playlist.ID),
					Name:  playlist.Name,
					Owner: playlist.Owner.ID,
				}
			}

			f, err := os.OpenFile(playlistListConf.csv, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
			if err != nil {
				return err
			}

			defer f.Close()

			err = gocsv.MarshalFile(rows, f)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

var purgePlaylistCmd = &cobra.Command{
	Use:   "purge",
	Short: "purge playlists marked as delete",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		if playlistPurgeConf.csv == "" {
			return errors.New("--csv argument is required")
		}

		zap.L().Info("purging playlists")
		spClient, err := getClient(ctx)
		if err != nil {
			return err
		}

		f, err := os.Open(playlistPurgeConf.csv)
		if err != nil {
			return err
		}

		defer f.Close()

		rows := []playlistRow{}
		err = gocsv.UnmarshalFile(f, &rows)
		if err != nil {
			return err
		}

		toPurge := make([]spotify.ID, 0, len(rows))
		for _, row := range rows {
			if row.Delete {
				toPurge = append(toPurge, spotify.ID(row.ID))
			}
		}

		zap.L().Info("found", zap.Int("playlists", len(rows)), zap.Int("to-purge", len(toPurge)))

		if playlistPurgeConf.dryrun {
			zap.L().Info("dryrun, not purging")
			return nil
		}

		bar := progressbar.Default(int64(len(toPurge)))

		wp := workerpool.New(5)

		for _, playlist := range toPurge {
			playlist := playlist
			wp.Submit(func() {
				//nolint
				defer bar.Add(1)
				err := spClient.UnfollowPlaylist(ctx, playlist)
				if err != nil {
					zap.L().Error("error purging playlist", zap.String("id", string(playlist)), zap.Error(err))
				}
			})
		}

		wp.StopWait()

		return nil
	},
}
