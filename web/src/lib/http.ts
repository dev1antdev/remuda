// Every request goes through here: the response check, JSON in and out, the machine on the query string.

// check turns a failed response into an Error with the server's message; a 401 means the cookie is gone, so the page reloads into sign-in
export async function check(res: Response): Promise<Response> {
  if (res.status === 401) {
    location.reload();
    throw new Error("unauthorized");
  }
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* keep statusText */
    }
    throw new Error(msg);
  }
  return res;
}

export const getJSON = <T,>(path: string, init?: RequestInit): Promise<T> =>
  fetch(path, init)
    .then(check)
    .then((r) => r.json() as Promise<T>);

export async function sendJSON<T = unknown>(method: "POST" | "PUT" | "DELETE" | "PATCH", url: string, body?: unknown): Promise<T> {
  const res = await check(
    await fetch(url, {
      method,
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    }),
  );
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

export function withMachine(path: string, machineId: string): string {
  return `${path}${path.includes("?") ? "&" : "?"}machine=${encodeURIComponent(machineId)}`;
}
