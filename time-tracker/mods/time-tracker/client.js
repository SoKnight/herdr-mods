// Talks to the collector through its CLI.
import { exec, appHome } from "herdr";
import { parseResult } from "./format.js";

const exe = () => `${appHome()}/mods/time-tracker/bin/herdr-time.exe`;

export async function call(args) {
  try {
    return parseResult(await exec(exe(), [...args, "--json"], { timeout: 10000 }));
  } catch (e) {
    // No exe (not deployed yet) is the same as a collector that isn't running.
    return /not found|no such|cannot find|os error 2/i.test(String(e?.message ?? e)) ? { down: true } : { error: String(e?.message ?? e) };
  }
}
