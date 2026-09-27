package control

import (
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in end-to-end test: isolated SQLite/storage, real PHP Laravel server,
// certificate-verified HTTPS proxy, real agent executable and Oto output.
func TestLaravelAudioEndToEnd(t *testing.T) {
	root, php, binary, mp3 := os.Getenv("ENGINE_LARAVEL_ROOT"), os.Getenv("ENGINE_PHP"), os.Getenv("ENGINE_E2E_BINARY"), os.Getenv("MUSIC_ENGINE_TEST_MP3")
	if root == "" || php == "" || binary == "" || mp3 == "" {
		t.Skip("set ENGINE_LARAVEL_ROOT, ENGINE_PHP, ENGINE_E2E_BINARY, MUSIC_ENGINE_TEST_MP3")
	}
	for _, mode := range []string{"stereo", "mono"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			storage := filepath.Join(dir, "storage")
			for _, p := range []string{"app/private/songs", "framework/cache/data", "framework/sessions", "framework/views", "logs"} {
				if err := os.MkdirAll(filepath.Join(storage, p), 0700); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(mp3)
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(storage, "app/private/songs/demo.mp3"), data, 0600)
			db := filepath.Join(dir, "database.sqlite")
			os.WriteFile(db, nil, 0600)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			address := listener.Addr().String()
			listener.Close()
			upstream, _ := url.Parse("http://" + address)
			proxy := httputil.NewSingleHostReverseProxy(upstream)
			var audioRequests, configRequests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/config") {
					configRequests.Add(1)
				}
				if strings.Contains(r.URL.Path, "/audio/") {
					if r.Header.Get("Authorization") != "" {
						t.Error("device credential sent to media")
					}
					if audioRequests.Add(1) == 1 {
						time.Sleep(2100 * time.Millisecond)
					}
				}
				proxy.ServeHTTP(w, r)
			}))
			defer server.Close()
			ca := filepath.Join(dir, "ca.pem")
			os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
			env := append(os.Environ(), "APP_ENV=testing", "APP_DEBUG=false", "APP_KEY=base64:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", "APP_URL="+server.URL, "ENGINE_MEDIA_URL="+server.URL, "ENGINE_AUDIO_URL_SECONDS=1", "ENGINE_WEBSOCKET_URL=", "ENGINE_POLL_INTERVAL_SECONDS=1", "DB_CONNECTION=sqlite", "DB_DATABASE="+db, "DB_URL=", "CACHE_STORE=array", "SESSION_DRIVER=array", "QUEUE_CONNECTION=sync", "BROADCAST_CONNECTION=log", "LARAVEL_STORAGE_PATH="+storage, "ENGINE_TEST_ROOT="+root)
			command := func(exe string, args ...string) *exec.Cmd {
				c := exec.Command(exe, args...)
				c.Dir = root
				c.Env = env
				return c
			}
			if out, err := command(php, "artisan", "migrate", "--force", "--no-interaction").CombinedOutput(); err != nil {
				t.Fatalf("isolated migrate: %v %s", err, out)
			}
			router := filepath.Join(dir, "router.php")
			os.WriteFile(router, []byte(`<?php $_SERVER['HTTPS']='on'; require getenv('ENGINE_TEST_ROOT').'/public/index.php';`), 0600)
			web := command(php, "-S", address, router)
			webLog, _ := os.Create(filepath.Join(dir, "php.log"))
			defer webLog.Close()
			web.Stderr = webLog
			if err := web.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { web.Process.Kill(); web.Wait() }()
			fixture := filepath.Join(dir, "fixture.php")
			os.WriteFile(fixture, []byte(laravelAudioFixture), 0600)
			call := func(args ...string) map[string]any {
				t.Helper()
				out, err := command(php, append([]string{fixture}, args...)...).Output()
				if err != nil {
					t.Fatalf("fixture %s failed: %v", args[0], err)
				}
				var v map[string]any
				if err := json.Unmarshal(out, &v); err != nil {
					t.Fatal("invalid fixture response", err)
				}
				return v
			}
			init := call("init")
			agent := command(binary)
			agent.Env = append(env, "ENGINE_SERVER="+server.URL, "ENGINE_TOKEN="+init["token"].(string), "ENGINE_STATE_DIR="+filepath.Join(dir, "state"), "ENGINE_CA_FILE="+ca, "ENGINE_PROFILE=shared_stereo_mp3", "ENGINE_CHANNEL_MODE="+mode)
			agentLog, _ := os.Create(filepath.Join(dir, "agent.log"))
			defer agentLog.Close()
			agent.Stderr = agentLog
			if err := agent.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { agent.Process.Kill(); agent.Wait() }()
			wantVolume := float64(10)
			waitFor := func(want string) {
				t.Helper()
				deadline := time.Now().Add(20 * time.Second)
				for time.Now().Before(deadline) {
					status := call("status")
					if status["state"] == want && status["volume"] == wantVolume && (status["outcome"] == "succeeded" || (want == "stopped" && status["outcome"] == nil)) {
						if status["mode"] != mode {
							t.Fatalf("mode: %v", status["mode"])
						}
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
				t.Fatalf("Laravel did not observe %s (audio=%d config=%d)", want, audioRequests.Load(), configRequests.Load())
			}
			waitFor("stopped")
			call("command", "play")
			waitFor("playing")
			if audioRequests.Load() != 2 || configRequests.Load() < 2 {
				t.Fatal("expired signed URL was not renewed exactly once")
			}
			call("command", "pause")
			waitFor("paused")
			call("volume", "45")
			wantVolume = 45
			waitFor("paused")
			call("command", "resume")
			waitFor("playing")
			call("command", "stop")
			waitFor("stopped")
			if audioRequests.Load() != 2 {
				t.Fatal("pause/resume/volume downloaded again")
			}
			call("command", "next")
			waitFor("playing")
			call("command", "previous")
			waitFor("playing")
			call("command", "play", "2")
			waitFor("playing")
			if status := call("status"); status["song"] != "2" {
				t.Fatalf("specific song not reported: %v", status)
			}
			call("command", "stop")
			waitFor("stopped")
			t.Logf("Laravel + signed HTTPS + agent + Oto verified in %s mode", mode)
		})
	}
}

const laravelAudioFixture = `<?php
require getenv('ENGINE_TEST_ROOT').'/vendor/autoload.php';
$app = require getenv('ENGINE_TEST_ROOT').'/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
use App\Models\Engine;
use App\Models\Playlist;
use App\Models\Song;
use App\Models\Zone;
use App\Services\EngineConfiguration;
if ($argv[1] === 'init') {
    $engine = Engine::factory()->create();
    $token = $engine->issueToken();
    $playlist = Playlist::create(['business_id'=>$engine->business_id,'name'=>'Integration']);
    $song = Song::create(['title'=>'Fixture','file_path'=>'songs/demo.mp3']);
    $playlist->songs()->attach($song,['position'=>1]);
    $second = Song::create(['title'=>'Second','file_path'=>'songs/demo.mp3']);
    $playlist->songs()->attach($second,['position'=>2]);
    Zone::create(['business_id'=>$engine->business_id,'engine_id'=>$engine->id,'playlist_id'=>$playlist->id,'name'=>'A','volume'=>10]);
    echo json_encode(['token'=>$token]);
} else {
    $engine = Engine::firstOrFail();
    $zone = $engine->zones()->firstOrFail();
    if ($argv[1] === 'command') {
        $command = app(EngineConfiguration::class)->enqueue($engine,(string)$zone->id,$argv[2],$argv[3] ?? null);
        echo json_encode(['id'=>$command->id]);
    } elseif ($argv[1] === 'volume') {
        $zone->update(['volume'=>(int)$argv[2]]);
        echo '{}';
    } else {
        $state = $engine->observed_state['zones'][0] ?? [];
        echo json_encode(['state'=>$state['state'] ?? null,'song'=>$state['song_id'] ?? null,'mode'=>$state['channel_mode'] ?? null,'volume'=>$state['volume'] ?? null,
            'outcome'=>$engine->latestCommand?->result['outcome'] ?? null]);
    }
}
`
