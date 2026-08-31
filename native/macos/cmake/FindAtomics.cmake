# Apple arm64 has lock-free C++ atomics in the target architecture and does
# not provide a separate libatomic. This avoids a false-negative probe from
# SDK-only build environments.
set(Atomics_FOUND TRUE)
set(ATOMICS_FOUND TRUE)
set(ATOMICS_LIBRARIES "")
