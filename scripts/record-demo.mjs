import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import http from "node:http";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const output = path.join(root, "docs", "media");
const temporary = await mkdtemp(path.join(os.tmpdir(), "goman-demo-"));
const server = http.createServer((request, response) => {
  response.writeHead(200, {
    "Content-Type": "application/json",
    "X-Request-Id": "req_demo_1042",
  });
  response.end(JSON.stringify({
    id: "order_1042",
    status: "confirmed",
    customer: "Ada Lovelace",
    items: [{ name: "Mechanical keyboard", quantity: 1, price: 89 }],
    total: 89,
    currency: "USD",
  }));
});

function run(command, args, env = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: root,
      env: { ...process.env, ...env },
      stdio: "inherit",
      timeout: 120_000,
    });
    child.once("error", reject);
    child.once("exit", (code, signal) => {
      if (code === 0) resolve();
      else reject(new Error(`${command} failed: ${signal ?? code}`));
    });
  });
}

try {
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const endpoint = `http://127.0.0.1:${server.address().port}`;
  const reservation = net.createServer();
  reservation.listen(0, "127.0.0.1");
  await once(reservation, "listening");
  const webhook = `127.0.0.1:${reservation.address().port}`;
  await new Promise(resolve => reservation.close(resolve));
  await mkdir(path.join(temporary, ".goman"));
  await mkdir(output, { recursive: true });
  await writeFile(path.join(temporary, ".goman", "collections.json"), JSON.stringify([
    {
      name: "Local shop API",
      requests: [
        { name: "Get order #1042", method: "GET", url: `${endpoint}/orders/1042`, headers: { Accept: "application/json" }, body: "" },
        { name: "Create an order", method: "POST", url: `${endpoint}/orders`, headers: { "Content-Type": "application/json" }, body: JSON.stringify({ product: "Mechanical keyboard", quantity: 1 }, null, 2) },
        { name: "Service health", method: "GET", url: `${endpoint}/health`, headers: {}, body: "" },
      ],
    },
  ], null, 2));
  const binary = path.join(temporary, "goman");
  const cast = path.join(output, "goman.cast");
  const animation = path.join(output, "goman.gif");
  await run("go", ["build", "-o", binary, "."]);
  await run("expect", ["-c", String.raw`
    set timeout 20
    log_user 0
    set command {exec "$GOMAN_DEMO_BINARY" --workspace "$GOMAN_DEMO_WORKSPACE" --webhook-address "$GOMAN_DEMO_WEBHOOK"}
    spawn -noecho asciinema rec --quiet --overwrite --return --output-format asciicast-v2 --capture-env TERM --window-size 120x36 --title "goman: requests and webhooks" --command $command $env(GOMAN_DEMO_CAST)
    expect {
      "GOMAN" {}
      timeout { puts stderr "goman did not start"; exit 1 }
      eof { puts stderr "goman exited before startup"; exit 1 }
    }
    after 1000
    send -- "\033\[Z\033\[Z\033\[Z\r"
    after 300
    send -- "\023"
    expect {
      "confirmed" {}
      timeout { puts stderr "response was not displayed"; exit 1 }
      eof { puts stderr "goman exited before response"; exit 1 }
    }
    after 3000
    send -- "\027"
    after 500
    send -- "\0333"
    after 500
    exec curl --fail --silent --show-error --max-time 5 -X POST "http://$env(GOMAN_DEMO_WEBHOOK)/events/orders" -H "Content-Type: application/json" -d {{"event":"order.shipped","order_id":"order_1042","carrier":"Postal Service","tracking":"DEMO-1042"}}
    expect {
      "order.shipped" {}
      timeout { puts stderr "webhook was not displayed"; exit 1 }
      eof { puts stderr "goman exited before webhook"; exit 1 }
    }
    after 3500
    send -- "\r"
    after 1500
    send -- "\021"
    after 500
    send -- "\r"
    expect eof
    set result [wait]
    exit [lindex $result 3]
  `], {
    TERM: "xterm-256color",
    GOMAN_DEMO_BINARY: binary,
    GOMAN_DEMO_WORKSPACE: temporary,
    GOMAN_DEMO_WEBHOOK: webhook,
    GOMAN_DEMO_CAST: cast,
  });
  const events = (await readFile(cast, "utf8")).trim().split("\n").map(line => JSON.parse(line));
  assert.equal(events[0].version, 2);
  assert.equal(events[0].width, 120);
  assert.equal(events[0].height, 36);
  const terminalOutput = events.slice(1).filter(event => event[1] === "o").map(event => event[2]).join("");
  assert.ok(terminalOutput.includes("confirmed"), "Recording has no HTTP response");
  assert.ok(terminalOutput.includes("order.shipped"), "Recording has no webhook");
  const previewEnd = events.at(-1)[0] - 0.8;
  await run("agg", ["--font-size", "16", "--fps-cap", "10", "--idle-time-limit", "10", "--last-frame-duration", "1", "--select", `0..${previewEnd}`, cast, animation]);
  for (const [name, token] of [["response", "confirmed"], ["webhooks", "order.shipped"]]) {
    let rendered = "";
    const event = events.slice(1).find(event => {
      if (event[1] === "o") rendered += event[2];
      return rendered.includes(token);
    });
    const seconds = String(event[0] + 2.5);
    await run("ffmpeg", ["-hide_banner", "-loglevel", "error", "-y", "-i", animation, "-ss", seconds, "-frames:v", "1", "-update", "1", path.join(output, `${name}.png`)]);
  }
  console.log(`Created recording, preview, and screenshots in ${output}`);
} finally {
  server.closeAllConnections();
  await new Promise(resolve => server.close(resolve));
  await rm(temporary, { recursive: true, force: true });
}