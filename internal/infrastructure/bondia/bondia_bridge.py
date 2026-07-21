import sys
import json
import urllib.request
import re
import datetime

def log_trace(msg):
    print(f"[BonDia API Trace] {msg}", file=sys.stderr)

def fetch_content(url, headers=None, timeout=10):
    if headers is None:
        headers = {'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'}
    req = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.read().decode('utf-8', errors='ignore')
    except Exception as e:
        log_trace(f"Error fetching URL {url}: {str(e)}")
        return None

def get_wikipedia_events():
    now = datetime.datetime.now()
    month = f"{now.month:02d}"
    day = f"{now.day:02d}"
    url = f"https://en.wikipedia.org/api/rest_v1/feed/onthisday/all/{month}/{day}"
    
    log_trace(f"Fetching Wikipedia On This Day events for {month}/{day}...")
    headers = {'User-Agent': 'FamiliarAssistant/1.0 (Contact: ezapaterm@gmail.com)'}
    raw_json = fetch_content(url, headers=headers)
    if not raw_json:
        return []
        
    try:
        data = json.loads(raw_json)
        events = data.get('selected', [])
        results = []
        for e in events[:8]:
            year = str(e.get('year', ''))
            text = e.get('text', '')
            link = ""
            if 'pages' in e and len(e['pages']) > 0:
                link = e['pages'][0].get('content_urls', {}).get('desktop', {}).get('page', '')
            if text and link:
                results.append({
                    "type": "efemeride",
                    "year": year,
                    "title": f"Efemèride històrica de l'any {year}",
                    "description": text,
                    "link": link
                })
        log_trace(f"Successfully loaded {len(results)} Wikipedia events.")
        return results
    except Exception as e:
        log_trace(f"Error parsing Wikipedia JSON: {str(e)}")
        return []

def get_rss_items(url, item_type):
    log_trace(f"Fetching RSS feed from {url} for type '{item_type}'...")
    content = fetch_content(url)
    if not content:
        return []
        
    items = re.findall(r'<item>(.*?)</item>', content, re.DOTALL)
    results = []
    for item in items:
        title_match = re.search(r'<title>(.*?)</title>', item, re.DOTALL)
        link_match = re.search(r'<link>(.*?)</link>', item, re.DOTALL)
        desc_match = re.search(r'<description>(.*?)</description>', item, re.DOTALL)
        
        title = title_match.group(1).strip() if title_match else ''
        link = link_match.group(1).strip() if link_match else ''
        desc = desc_match.group(1).strip() if desc_match else ''
        
        # Clean CDATA
        title = re.sub(r'<!\[CDATA\[(.*?)\]\]>', r'\1', title, flags=re.DOTALL).strip()
        link = re.sub(r'<!\[CDATA\[(.*?)\]\]>', r'\1', link, flags=re.DOTALL).strip()
        desc = re.sub(r'<!\[CDATA\[(.*?)\]\]>', r'\1', desc, flags=re.DOTALL).strip()
        
        # Remove HTML tags from description
        desc = re.sub(r'<[^>]*>', '', desc).strip()
        desc = re.sub(r'\s+', ' ', desc)
        
        if title and link:
            results.append({
                "type": item_type,
                "year": "",
                "title": title,
                "description": desc[:300] + ("..." if len(desc) > 300 else ""),
                "link": link
            })
            
    log_trace(f"Successfully loaded {len(results)} items from {url}.")
    return results[:8]

def main():
    log_trace("Iniciant bridge de recollida de notícies i efemèrides per a /bondia...")
    
    # 1. Wikipedia Events
    wiki_events = get_wikipedia_events()
    
    # 2. Vilaweb (Catalan News)
    vilaweb_news = get_rss_items("https://www.vilaweb.cat/rss/", "catalunya")
    
    # 3. Good News Network (Positive/Global News)
    good_news = get_rss_items("https://www.goodnewsnetwork.org/category/news/feed/", "bones_noticies")
    
    combined = wiki_events + vilaweb_news + good_news
    
    # Output to stdout as a single JSON
    print(json.dumps(combined, ensure_ascii=False))

if __name__ == "__main__":
    main()
