# Product Requirements Document (PRD)

## 1. Project Overview
This project aims to build a functional web crawler and a real-time search engine completely from scratch. The system will prioritize architectural sensibility, robust concurrency management, and "Human-in-the-Loop" (HITL) verification. To demonstrate low-level systems engineering, the project strictly prohibits the use of high-level scraping libraries, relying instead on language-native HTTP and parsing modules.

## 2. Core Objectives
* **Architectural Sensibility:** Design a modular system where the indexer, search engine, and dashboard operate independently but communicate efficiently.
* **Concurrency Management:** Safely handle high-throughput, multi-threaded network I/O and shared memory state without data corruption.
* **Human-in-the-Loop (HITL):** Implement verification checkpoints where a human operator can audit crawl quality, adjust parameters, or approve domain constraints.

## 3. Technical Constraints & Guidelines
* **Native Focus:** Must use language-native functionality (e.g., `net/http` in Go, `urllib` in Python, or equivalent) for network requests and basic string/HTML parsing.
* **Zero Magic:** High-level abstractions like Scrapy, Beautiful Soup, or Selenium are strictly prohibited.
* **Thread Safety:** AI-assisted design of concurrent data structures (Mutexes, Channels, Concurrent Maps) is required to ensure memory safety.

---

## 4. Functional Requirements

### 4.1 Indexer Engine
The Indexer is responsible for discovering, fetching, and processing web pages.
* **Recursive Crawling:** Must be able to initiate from an origin seed URL and recursively crawl links up to a strictly enforced maximum depth of `k`.
* **Uniqueness & Cycle Prevention:** Must implement a thread-safe "Visited" set to ensure no single URL is fetched or processed more than once.
* **Back-Pressure Management:** The system must proactively manage its own load. It should throttle network requests based on maximum rate limits or current queue depth to prevent memory overflow and avoid overwhelming target servers.

### 4.2 Query & Search Engine
The Searcher allows users to query the parsed data simultaneously while the crawler is running.
* **Live Indexing:** Read and write operations must be heavily decoupled. The search function must execute accurately without locking up the active indexer.
* **Query Output:** Search results must return a standard schema of triples: `(relevant_url, origin_url, depth)`.
* **Relevancy Heuristics:** Implement a foundational ranking algorithm based on keyword frequency, title matching, or term proximity to sort the triples logically.
* **Concurrent Data Structures:** The inverted index (or primary search structure) must use read-write mutexes (`RWMutex`) or channel-based worker pools to allow concurrent user queries while background threads update the index.

### 4.3 UI/CLI Dashboard
A real-time monitoring interface (terminal-based or simple web UI) to observe the system's live state.
* **Core Metrics:**
    1.  **Current Indexing Progress:** Ratio of URLs successfully processed versus URLs currently sitting in the frontier queue.
    2.  **Current Queue Depth:** An absolute count of pending URLs awaiting fetch.
    3.  **Back-pressure / Throttling Status:** Visual indicators showing if the crawler is currently running at normal capacity, throttled, or paused due to load.

### 4.4 Resiliency & State Management
* **Interruption Recovery:** The system must gracefully handle interrupts (e.g., `SIGINT`). 
* **State Persistence:** The frontier queue, visited set, and current index must be periodically checkpointed to disk (e.g., using a Write-Ahead Log or SQLite).
* **Seamless Resumption:** Upon restart, the system must read the persistent state and resume the crawl exactly where it left off, without revisiting old pages or losing the current depth context.

### 4.5 Human-in-the-Loop (HITL) Verification
* **Quality Audits:** The system must allow an operator to pause the crawler, sample the latest parsed text/tokens, and verify that the native HTML parsing is functioning correctly before authorizing a deeper crawl.
* **Intervention:** The dashboard must allow a human to manually drop specific domains from the queue or adjust the back-pressure limits on the fly.

---

## 5. Future Considerations (Out of Scope for v1)
* Distributed crawling across multiple physical machines.
* Advanced NLP-based relevancy models (e.g., TF-IDF or embeddings).
* JavaScript rendering support.
