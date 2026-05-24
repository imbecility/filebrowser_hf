import { fetchURL } from "./utils";

export type RepoType = "model" | "dataset" | "space";

export interface UploadRequest {
    repoId: string;
    repoType: RepoType;
    paths: string[];
    pathInRepoPrefix?: string;
    commitMessage?: string;
    token?: string;
}

export interface HFProgressEvent {
    action: "discover" | "start" | "update" | "done" | "complete" | "error";
    message?: string;
    path?: string;
    totalBytes?: number;
    bytesDone?: number;
}

export async function getTokenStatus(): Promise<boolean> {
    const res = await fetchURL("/api/hf/token-status", {});
    if (!res.ok) throw new Error("Failed to check token status");
    const data = (await res.json()) as { hasToken: boolean };
    return data.hasToken;
}

/**
 * Starts SSE download stream. Each progress event is passed to onProgress.
 * The function resolves after the stream is closed (complete or end of connection).
 * Throws an error only on network/HTTP failures - SSE error/complete events
 * processed in onProgress on the component side.
 */
export async function upload(
    request: UploadRequest,
    onProgress: (event: HFProgressEvent) => void,
    signal?: AbortSignal
): Promise<void> {
    const res = await fetchURL("/api/hf/upload", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request),
        signal,
    });

    if (!res.ok) {
        const text = await res.text();
        throw new Error(`Server error ${res.status}: ${text}`);
    }

    const reader = res.body!.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    try {
        while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            // Accumulate chunks - an SSE event may arrive split into TCP packets
            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split("\n");
            // The last line may be incomplete - keep it in a buffer
            buffer = lines.pop() ?? "";

            for (const line of lines) {
                if (!line.startsWith("data: ")) continue;
                try {
                    const event: HFProgressEvent = JSON.parse(line.slice(6));
                    onProgress(event);
                } catch {
                    // skip broken lines
                }
            }
        }
    } finally {
        reader.releaseLock();
    }
}
