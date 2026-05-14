import net from "node:net";

const host = process.argv[2] || "127.0.0.1";
const startPort = parsePort(process.argv[3], 5173);
const maxPort = parsePort(process.argv[4], startPort + 20);

for (let port = startPort; port <= maxPort; port += 1) {
  if (await isPortFree(host, port)) {
    console.log(String(port));
    process.exit(0);
  }
}

console.error(`No free desktop renderer port found on ${host} from ${startPort} to ${maxPort}.`);
process.exit(1);

function parsePort(raw, fallback) {
  const port = Number(raw);
  return Number.isInteger(port) && port > 0 && port <= 65535 ? port : fallback;
}

function isPortFree(host, port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once("error", () => resolve(false));
    server.listen(port, host, () => {
      server.close(() => resolve(true));
    });
  });
}
