# Image Generation Tool Design

## Goal

Provide a public `/image-gen` page for operations staff to generate images by pasting a valid platform API key.

## Scope

The first version is a frontend-only tool. It calls the existing OpenAI-compatible gateway endpoint `POST /v1/images/generations` and relies on the current backend for API key authentication, group image-generation permission checks, moderation, routing, billing, and usage logging.

## User Experience

The page is reachable without login. Users enter an API key, prompt, size, and image count, then submit. The page shows generated images, revised prompts when present, and action buttons for download and copying the image Data URL.

Generated results are also saved to local browser history. History stores prompt, model, size, created time, and generated image data URLs. It keeps the most recent 20 records and supports deleting one record or clearing all history.

## Data Flow

1. User opens `/image-gen`.
2. User enters API key and prompt.
3. Frontend sends `POST /v1/images/generations` with `Authorization: Bearer <api-key>`.
4. Backend returns OpenAI Images API JSON.
5. Frontend renders image results and records a local history entry.

## Error Handling

The page validates that API key and prompt are present before calling the gateway. Network and gateway errors are shown inline. When the response does not contain usable image data, the page shows a clear empty-result error.

## Storage

History uses `localStorage` key `image-gen-history-v1`. API keys are not saved. If local storage is unavailable or full, generation still works, but history may not persist.

## Testing

Add Vue tests that verify the page sends the correct gateway request and records generated image history locally.
