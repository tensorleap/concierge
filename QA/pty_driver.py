#!/usr/bin/env python3
from __future__ import annotations

import errno
import fcntl
import os
import pty
import select
import signal
import struct
import subprocess
import termios
import threading
import time
from pathlib import Path
from typing import Mapping, Sequence


class PTYDriver:
    """Run a subprocess inside a pseudo terminal and shuttle bytes in both directions."""

    def __init__(self, *, encoding: str = "utf-8", columns: int = 120, rows: int = 40) -> None:
        self.encoding = encoding
        self.columns = columns
        self.rows = rows
        self._master_fd: int | None = None
        self._process: subprocess.Popen[bytes] | None = None
        self._buffer = ""
        self._buffer_lock = threading.Lock()
        self._reader_stop = threading.Event()
        self._reader_thread: threading.Thread | None = None

    def start(
        self,
        command: Sequence[str],
        *,
        cwd: str | os.PathLike[str] | None = None,
        env: Mapping[str, str] | None = None,
    ) -> None:
        if self._process is not None:
            raise RuntimeError("PTYDriver is already running")
        if not command:
            raise ValueError("command is required")

        master_fd, slave_fd = pty.openpty()
        self._configure_pty(master_fd)
        self._set_window_size(slave_fd)
        self._process = subprocess.Popen(
            list(command),
            stdin=slave_fd,
            stdout=slave_fd,
            stderr=slave_fd,
            cwd=str(Path(cwd).resolve()) if cwd is not None else None,
            env=dict(env) if env is not None else None,
            close_fds=True,
        )
        os.close(slave_fd)
        self._master_fd = master_fd
        self._buffer = ""
        self._reader_stop.clear()
        self._reader_thread = threading.Thread(target=self._reader_loop, name="qa-loop-pty-reader", daemon=True)
        self._reader_thread.start()

    def read(self, *, timeout: float = 0.2, max_bytes: int = 65536) -> str:
        deadline = time.monotonic() + max(timeout, 0.0)
        while True:
            chunk = self.drain(max_bytes=max_bytes)
            if chunk:
                return chunk
            if timeout <= 0 or time.monotonic() >= deadline:
                return ""
            if not self.is_running() and self._buffer_is_empty():
                return ""
            time.sleep(min(0.05, max(deadline - time.monotonic(), 0.0)))

    def read_until_quiet(
        self,
        *,
        quiet_period: float = 0.35,
        hard_timeout: float = 2.0,
        max_bytes: int = 262144,
    ) -> str:
        chunks: list[str] = []
        started_at = time.monotonic()
        quiet_deadline = started_at + quiet_period
        bytes_seen = 0

        while True:
            now = time.monotonic()
            remaining = hard_timeout - (now - started_at)
            if remaining <= 0:
                break
            chunk = self.read(timeout=min(0.1, max(remaining, 0.0)), max_bytes=max_bytes - bytes_seen)
            if chunk:
                chunks.append(chunk)
                bytes_seen += len(chunk.encode(self.encoding, errors="replace"))
                quiet_deadline = time.monotonic() + quiet_period
                if bytes_seen >= max_bytes:
                    break
                continue
            if time.monotonic() >= quiet_deadline:
                break
        return "".join(chunks)

    def drain(self, *, max_bytes: int = 65536) -> str:
        with self._buffer_lock:
            if not self._buffer:
                return ""
            chunk = self._buffer[:max_bytes]
            self._buffer = self._buffer[max_bytes:]
            return chunk

    def send(self, text: str, *, append_newline: bool = False) -> None:
        if self._master_fd is None:
            raise RuntimeError("PTYDriver is not running")
        payload = text + ("\n" if append_newline else "")
        os.write(self._master_fd, payload.encode(self.encoding))

    def is_running(self) -> bool:
        return self._process is not None and self._process.poll() is None

    @property
    def returncode(self) -> int | None:
        if self._process is None:
            return None
        return self._process.poll()

    def stop(self, *, terminate_timeout: float = 2.0, kill_timeout: float = 1.0) -> int | None:
        process = self._process
        if process is None:
            self._stop_reader()
            self._close_master()
            return None
        if process.poll() is None:
            process.send_signal(signal.SIGTERM)
            try:
                process.wait(timeout=terminate_timeout)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=kill_timeout)
        code = process.returncode
        self._process = None
        self._stop_reader()
        self._close_master()
        return code

    def _reader_loop(self) -> None:
        while not self._reader_stop.is_set():
            master_fd = self._master_fd
            if master_fd is None:
                return
            try:
                ready, _, _ = select.select([master_fd], [], [], 0.1)
            except OSError:
                return
            if not ready:
                if self._process is not None and self._process.poll() is not None:
                    return
                continue
            try:
                payload = os.read(master_fd, 65536)
            except BlockingIOError:
                continue
            except OSError as exc:
                if exc.errno == errno.EIO:
                    return
                raise
            if not payload:
                if self._process is not None and self._process.poll() is not None:
                    return
                continue
            chunk = payload.decode(self.encoding, errors="replace")
            with self._buffer_lock:
                self._buffer += chunk

    def _buffer_is_empty(self) -> bool:
        with self._buffer_lock:
            return self._buffer == ""

    def _stop_reader(self) -> None:
        self._reader_stop.set()
        if self._reader_thread is not None:
            self._reader_thread.join(timeout=0.5)
            self._reader_thread = None

    def _configure_pty(self, master_fd: int) -> None:
        os.set_blocking(master_fd, False)
        flags = fcntl.fcntl(master_fd, fcntl.F_GETFL)
        fcntl.fcntl(master_fd, fcntl.F_SETFL, flags | os.O_NONBLOCK)

    def _set_window_size(self, slave_fd: int) -> None:
        winsize = struct.pack("HHHH", self.rows, self.columns, 0, 0)
        fcntl.ioctl(slave_fd, termios.TIOCSWINSZ, winsize)

    def _close_master(self) -> None:
        if self._master_fd is None:
            return
        os.close(self._master_fd)
        self._master_fd = None
