/** Capture the complete rendered table, including columns outside its scroll container. */
export async function captureTablePng(table: HTMLTableElement, signal: AbortSignal): Promise<Blob> {
  const { default: html2canvas } = await import('html2canvas')
  await document.fonts?.ready
  signal.throwIfAborted()

  const width = Math.ceil(Math.max(table.scrollWidth, table.getBoundingClientRect().width))
  const height = Math.ceil(Math.max(table.scrollHeight, table.getBoundingClientRect().height))
  if (!width || !height) throw new Error('The table has no rendered content')

  // Keep 100-row tables within canvas dimension/memory limits while preferring 2x output.
  const scale = Math.min(2, 16384 / width, 16384 / height, Math.sqrt(16_000_000 / (width * height)))
  const backgroundColor = document.documentElement.classList.contains('dark') ? '#111827' : '#ffffff'
  const canvas = await html2canvas(table, {
    backgroundColor,
    width,
    height,
    scale,
    logging: false,
    onclone: (_document, clonedTable) => {
      // Only adjust the clone: the user's current scroll position and layout stay intact.
      clonedTable.style.width = `${width}px`
      for (let parent = clonedTable.parentElement; parent; parent = parent.parentElement) {
        parent.style.overflow = 'visible'
        parent.scrollLeft = 0
        parent.scrollTop = 0
      }
    }
  })
  try {
    signal.throwIfAborted()
    const blob = await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(value => value ? resolve(value) : reject(new Error('PNG encoding failed')), 'image/png')
    })
    signal.throwIfAborted()
    return blob
  } finally {
    canvas.width = 0
    canvas.height = 0
  }
}
