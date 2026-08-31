# Apple platforms provide pthreads through libSystem. Some command-line-tools
# SDK/CMake combinations fail FindThreads' compile probe even though the
# pthread API and linker symbols are available.
set(Threads_FOUND TRUE)
set(THREADS_FOUND TRUE)
set(CMAKE_THREAD_LIBS_INIT "")
set(CMAKE_USE_PTHREADS_INIT TRUE)
set(THREADS_PTHREADS_INIT TRUE)
if(NOT TARGET Threads::Threads)
  add_library(Threads::Threads INTERFACE IMPORTED)
endif()
