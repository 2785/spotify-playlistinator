# spotify-playlistinator

A thing to manage spotify playlists in bulk

```
Util to edit playlists in bulk

Usage:
  spotify-playlistinator [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  liked       Manage liked tracks
  playlists   utils to manage entire playlists

Flags:
  -h, --help   help for spotify-playlistinator

Use "spotify-playlistinator [command] --help" for more information about a command.
```

## Install

On whatever platform you're on, get golang working (which can be easily done with [g](https://github.com/stefanmaric/g)). And

```shell
go install github.com/2785/spotify-playlistinator@latest
```

## Client ID

Head to spotify developer portal, create an app for your self, call it whatever. Go to settings of the app, add `http://localhost:8080/callback` into the allowed list of oauth callback URLs, hit save.

Copy the `ClientID` from the app page, and export the env var like so

```shell
export SPOTIFY_CLIENT_ID=abc234...
```

## Playlist management

Playlist management workflow is as follows, to list all your playlists, run

```shell
spotify-playlistinator playlists list --csv playlists.csv
```

This will output all the playlists you follow into a file called `playlists.csv`. The column in the csv called "delete" will be the instruction for the thing to purge that specific playlist.

You can proceed to drop this file into, say, for example, google sheet, to filter, sort, and mark things as to be deleted, and toss that back into the csv file (or a new file, your call), then to run the purge:

```shell
spotify-playlistinator playlists purge --csv playlists.csv
```

This will proceed to delete all items marked as delete in the csv file.

The purge comes with a `--dryrun` option in case you want a preview for how many things you're deleting

## Liked Tracks management

Literally the same thing with playlist management but for tracks
