package cmd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

var spotifyClient *spotify.Client
var clientMu sync.RWMutex

func getClient(ctx context.Context) (c *spotify.Client, err error) {
	clientMu.RLock()
	if spotifyClient != nil {
		c = spotifyClient
		clientMu.RUnlock()
		return
	}

	clientMu.RUnlock()
	clientMu.Lock()
	defer clientMu.Unlock()

	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	if clientID == "" {
		return nil, errors.New("SPOTIFY_CLIENT_ID not set")
	}

	cfg.ClientID = clientID

	tok, ok := func() (*oauth2.Token, bool) {
		// best effort cache retrieval
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, false
		}

		tokenFile := path.Join(home, ".playlistinator/token.json")

		bytes, err := ioutil.ReadFile(tokenFile)
		if err != nil {
			return nil, false
		}

		tok := &oauth2.Token{}
		err = json.Unmarshal(bytes, tok)
		if err != nil {
			return nil, false
		}

		tokSource := cfg.TokenSource(ctx, tok)
		tok, err = tokSource.Token()
		if err != nil {
			return nil, false
		}

		return tok, true
	}()

	if !ok {
		tok, err = authenticate(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to authenticate")
		}

		func() {
			// best effort cache creation
			home, err := os.UserHomeDir()
			if err != nil {
				return
			}

			tokenFile := path.Join(home, ".playlistinator/token.json")

			bytes, err := json.Marshal(tok)
			if err != nil {
				return
			}

			err = os.MkdirAll(path.Dir(tokenFile), os.ModePerm)
			if err != nil {
				return
			}

			f, err := os.OpenFile(tokenFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
			if err != nil {
				return
			}

			defer f.Close()

			_, err = f.Write(bytes)
			if err != nil {
				return
			}
		}()
	}

	// eitherway, we now have a token to use :)
	hc := cfg.Client(ctx, tok)
	c = spotify.New(hc, spotify.WithRetry(true))
	spotifyClient = c

	return
}

var cfg = &oauth2.Config{
	Endpoint: oauth2.Endpoint{
		AuthURL:  spotifyauth.AuthURL,
		TokenURL: spotifyauth.TokenURL,
	},
	RedirectURL: "http://localhost:8080/callback",
	Scopes: []string{
		spotifyauth.ScopePlaylistReadPrivate,
		spotifyauth.ScopePlaylistModifyPrivate,
		spotifyauth.ScopePlaylistReadCollaborative,
		spotifyauth.ScopePlaylistModifyPublic,
		spotifyauth.ScopeUserLibraryRead,
		spotifyauth.ScopeUserLibraryModify,
	},
}

func authenticate(ctx context.Context) (tok *oauth2.Token, err error) {
	state := uuid.New().String()
	challenge := generateCodeChallenge(100)
	hash := sha256.Sum256(challenge)
	hashedVerifier := base64.URLEncoding.EncodeToString(hash[:])

	hashedVerifier = strings.TrimRight(hashedVerifier, "=")

	authURL := cfg.AuthCodeURL(state, oauth2.SetAuthURLParam("code_challenge_method", "S256"), oauth2.SetAuthURLParam("code_challenge", hashedVerifier))

	shutdownChan := make(chan struct{})
	shutdownErrChan := make(chan error)

	var gotCode, gotState string

	router := mux.NewRouter()
	router.Handle("/callback", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		gotState = r.URL.Query().Get("state")
		gotCode = r.URL.Query().Get("code")
		shutdownChan <- struct{}{}
	}))

	srv := &http.Server{
		Handler: router,
		Addr:    "localhost:8080",
	}

	zap.L().Info("Please open the following URL in your browser to authenticate", zap.String("url", authURL))

	go func() {
		<-shutdownChan
		err := srv.Close()
		shutdownErrChan <- err
	}()

	err = srv.ListenAndServe()
	if err != http.ErrServerClosed {
		// Error starting or closing listener:
		return nil, errors.Wrapf(err, "failed to capture auth callback")
	}

	shutdownErr := <-shutdownErrChan
	if shutdownErr != nil {
		return nil, errors.Wrapf(shutdownErr, "failed to capture auth callback")
	}

	if gotState != state {
		return nil, errors.Errorf("state mismatch, expected %s, got %s", state, gotState)
	}

	token, err := cfg.Exchange(ctx, gotCode, oauth2.SetAuthURLParam("code_verifier", string(challenge)), oauth2.SetAuthURLParam("client_id", "60212408a4914e6da50e448a73fbe4ac"))

	if err != nil {
		return nil, errors.Wrapf(err, "failed to exchange auth code")
	}

	return token, nil
}

func generateCodeChallenge(l int) []byte {
	charSet := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-~"

	b := make([]byte, l)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charSet))))
		if err != nil {
			panic("just no please wtf")
		}
		b[i] = charSet[num.Int64()]
	}

	return b
}
