import { detectReceiptBounds } from '../lib/detectReceiptBounds'

self.onmessage = (event: MessageEvent<{ data: Uint8ClampedArray; width: number; height: number }>) => {
  const { data, width, height } = event.data
  const started = performance.now()
  try {
    self.postMessage({ rect: detectReceiptBounds(data, width, height), elapsedMs: performance.now() - started })
  } catch {
    self.postMessage({ rect: null, elapsedMs: performance.now() - started })
  }
}
