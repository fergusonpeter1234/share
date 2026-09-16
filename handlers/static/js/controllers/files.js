"use strict";

const chunkedUploadChunkSize = 50 * 1024 * 1024;

function uploadFormData(url, formData, progressFn) {
  return new Promise((resolve, reject) => {
    // We have to use XHR instead of fetch because fetch currently doesn't
    // support a mechanism for reporting upload progress.
    const xhr = new XMLHttpRequest();
    xhr.open("POST", url, true);
    xhr.setRequestHeader("Accept", "application/json");
    xhr.upload.addEventListener("progress", (event) => {
      if (event.lengthComputable) {
        if (progressFn) {
          progressFn(event.loaded, event.total);
        }
      }
    });
    xhr.addEventListener("loadend", () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(xhr.response);
      } else {
        if (xhr.responseText) {
          reject(xhr.responseText);
          return;
        }
        reject(xhr.statusText);
      }
    });
    xhr.addEventListener("error", () => {
      reject(
        "Failed to communicate with server" +
          (xhr.statusText ? `: ${xhr.statusText}` : "."),
      );
    });
    xhr.send(formData);
  })
    .then((raw) => {
      return Promise.resolve(JSON.parse(raw));
    })
    .then((data) => {
      if (!Object.prototype.hasOwnProperty.call(data, "id")) {
        throw new Error("Missing expected id field");
      }
      return Promise.resolve(data);
    });
}

async function startChunkedUpload(url, file, expirationTime, note) {
  let response;
  try {
    response = await fetch(url, {
      method: "POST",
      credentials: "same-origin",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        filename: file.name,
        contentType: file.type,
        expiration: expirationTime,
        note: note || "",
        size: file.size,
      }),
    });
  } catch (error) {
    throw new Error(
      "Failed to communicate with server" +
        (error.message ? `: ${error.message}` : "."),
    );
  }

  if (!response.ok) {
    const error = await response.text();
    throw error || response.statusText;
  }

  const data = await response.json();
  if (
    !Object.prototype.hasOwnProperty.call(data, "uploadId") ||
    !data.uploadId
  ) {
    throw new Error("Missing expected uploadId field");
  }
  return data.uploadId;
}

function uploadChunk(
  url,
  chunk,
  contentRange,
  bytesUploaded,
  bytesTotal,
  progressFn,
) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PATCH", url, true);
    xhr.setRequestHeader("Accept", "application/json");
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    xhr.setRequestHeader("Content-Range", contentRange);
    xhr.upload.addEventListener("progress", (event) => {
      if (event.lengthComputable && progressFn) {
        progressFn(bytesUploaded + event.loaded, bytesTotal);
      }
    });
    xhr.addEventListener("loadend", () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(xhr.responseText);
      } else {
        if (xhr.responseText) {
          reject(xhr.responseText);
          return;
        }
        reject(xhr.statusText);
      }
    });
    xhr.addEventListener("error", () => {
      reject(
        "Failed to communicate with server" +
          (xhr.statusText ? `: ${xhr.statusText}` : "."),
      );
    });
    xhr.send(chunk);
  }).then((raw) => {
    const data = JSON.parse(raw);
    if (!Object.prototype.hasOwnProperty.call(data, "offset")) {
      throw new Error("Missing expected offset field");
    }
    return data;
  });
}

async function uploadFileInChunks(
  file,
  expirationTime,
  note,
  progressFn,
  startURL,
  chunkURL,
) {
  const uploadID = await startChunkedUpload(
    startURL,
    file,
    expirationTime,
    note,
  );

  let bytesUploaded = 0;
  while (bytesUploaded < file.size) {
    const nextOffset = Math.min(
      bytesUploaded + chunkedUploadChunkSize,
      file.size,
    );
    const chunk = file.slice(bytesUploaded, nextOffset);
    const data = await uploadChunk(
      chunkURL(uploadID),
      chunk,
      `bytes ${bytesUploaded}-${nextOffset - 1}/${file.size}`,
      bytesUploaded,
      file.size,
      progressFn,
    );

    if (data.offset !== nextOffset) {
      throw new Error("Server accepted an unexpected upload offset");
    }
    bytesUploaded = nextOffset;

    if (bytesUploaded === file.size) {
      if (!Object.prototype.hasOwnProperty.call(data, "id")) {
        throw new Error("Missing expected id field");
      }
      return data;
    }
  }

  throw new Error("Chunked upload did not complete");
}

export async function uploadFile(file, expirationTime, note, progressFn) {
  if (file.size > chunkedUploadChunkSize) {
    return uploadFileInChunks(
      file,
      expirationTime,
      note,
      progressFn,
      "/api/entry/upload",
      (uploadID) => `/api/entry/upload/${encodeURIComponent(uploadID)}`,
    );
  }

  const formData = new FormData();
  formData.append("file", file);
  if (note) {
    formData.append("note", note);
  }
  return uploadFormData(
    `/api/entry?expiration=${encodeURIComponent(expirationTime)}`,
    formData,
    progressFn,
  );
}

export async function guestUploadFile(
  file,
  guestLinkID,
  expirationTime,
  progressFn,
) {
  if (file.size > chunkedUploadChunkSize) {
    const encodedGuestLinkID = encodeURIComponent(guestLinkID);
    return uploadFileInChunks(
      file,
      expirationTime,
      null,
      progressFn,
      `/api/guest/${encodedGuestLinkID}/upload`,
      (uploadID) =>
        `/api/guest/${encodedGuestLinkID}/upload/${encodeURIComponent(
          uploadID,
        )}`,
    );
  }

  const formData = new FormData();
  formData.append("file", file);
  return uploadFormData(
    `/api/guest/${guestLinkID}?expiration=${encodeURIComponent(
      expirationTime,
    )}`,
    formData,
    progressFn,
  );
}

export async function editFile(id, filename, expiration, note) {
  let payload = {
    filename,
    note,
  };
  if (expiration) {
    payload.expiration = expiration;
  }
  return fetch(`/api/entry/${encodeURIComponent(id)}`, {
    method: "PUT",
    credentials: "include",
    body: JSON.stringify(payload),
  })
    .then((response) => {
      if (!response.ok) {
        return response.text().then((error) => {
          return Promise.reject(error);
        });
      }
      return Promise.resolve();
    })
    .catch((error) => {
      if (error.message) {
        return Promise.reject(
          "Failed to communicate with server" +
            (error.message ? `: ${error.message}` : "."),
        );
      }
      return Promise.reject(error);
    });
}

export async function deleteFile(id) {
  return fetch(`/api/entry/${id}`, {
    method: "DELETE",
    credentials: "include",
  })
    .then((response) => {
      if (!response.ok) {
        return response.text().then((error) => {
          return Promise.reject(error);
        });
      }
      return Promise.resolve();
    })
    .catch((error) => {
      if (error.message) {
        return Promise.reject(
          "Failed to communicate with server" +
            (error.message ? `: ${error.message}` : "."),
        );
      }
      return Promise.reject(error);
    });
}
