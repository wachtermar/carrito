#!/usr/bin/env python3
"""Stateful local Alcampo-shaped server for Hermes end-to-end evaluations."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import threading
import unicodedata
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


def product(sku, name, size, price, keywords, ingredients="", allergens="", category="Despensa"):
    return {
        "productId": f"mock-{sku.lower()}",
        "retailerProductId": sku,
        "name": name,
        "brand": "Mock Mercado",
        "size": size,
        "price": {"amount": f"{price:.2f}", "currency": "EUR"},
        "unitPrice": {"amount": f"{price:.2f}", "currency": "EUR", "unit": "unit"},
        "available": True,
        "category": category,
        "ingredients": ingredients,
        "allergens": allergens,
        "images": [],
        "keywords": keywords,
    }


def unavailable_product(sku, name, size, price, keywords):
    item = product(sku, name, size, price, keywords, "ingrediente de prueba")
    item["available"] = False
    return item


CATALOG = [
    product("PASTA500", "Macarrones de trigo", "500 g", 1.35, "pasta macarrones trigo", "sémola de trigo", "contiene gluten"),
    product("GFPASTA400", "Pasta de maíz y arroz sin gluten", "400 g", 2.65, "pasta macarrones sin gluten maiz arroz", "harina de maíz, harina de arroz", ""),
    product("TOMATO800", "Tomate triturado", "800 g", 1.55, "tomate triturado salsa", "tomate", ""),
    product("SPINACH300", "Espinacas frescas", "300 g", 1.80, "espinaca verduras", "espinacas", "", "Verduras"),
    product("CHICKPEA400", "Garbanzos cocidos", "400 g", 0.95, "garbanzos cocidos legumbres", "garbanzos, agua, sal", ""),
    product("LENTIL400", "Lentejas cocidas", "400 g", 0.95, "lentejas cocidas legumbres", "lentejas, agua, sal", ""),
    product("TOFU400", "Tofu natural", "400 g", 2.45, "tofu proteina vegetal soja", "soja, agua", "contiene soja", "Vegetal"),
    product("CHICKEN600", "Pechuga de pollo", "600 g", 5.95, "pollo pechuga carne", "pechuga de pollo", "", "Carne"),
    product("SALMON500", "Lomos de salmón", "500 g", 8.50, "salmon pescado lomos", "salmón", "contiene pescado", "Pescado"),
    product("HAKE500", "Filetes de merluza", "500 g", 5.75, "merluza pescado filetes", "merluza", "contiene pescado", "Pescado"),
    product("RICE1000", "Arroz redondo", "1 kg", 1.60, "arroz grano", "arroz", ""),
    product("POTATO2K", "Patatas para cocinar", "2 kg", 2.90, "patata papas", "patata", "", "Verduras"),
    product("BROCCOLI500", "Brócoli", "500 g", 1.85, "brocoli verduras", "brócoli", "", "Verduras"),
    product("MIXVEG1K", "Verduras variadas congeladas", "1 kg", 2.60, "verduras mezcla congeladas", "zanahoria, judía verde, guisantes", ""),
    product("SOYMILK1L", "Bebida de soja sin azúcar", "1 L", 1.45, "leche bebida soja vegetal", "agua, soja", "contiene soja", "Bebidas"),
    product("LFMILK1L", "Leche sin lactosa", "1 L", 1.20, "leche sin lactosa", "leche de vaca", "contiene leche", "Lácteos"),
    product("COCOYOG4", "Yogur vegetal de coco", "4 x 125 g", 2.40, "yogur vegetal coco sin lactosa", "coco, agua", "", "Vegetal"),
    product("EGGS12", "Huevos camperos", "12 uds", 3.45, "huevos huevo", "huevo", "contiene huevo", "Huevos"),
    product("WRAP6", "Tortillas de trigo", "6 uds", 1.75, "tortilla wrap trigo", "harina de trigo, agua", "contiene gluten"),
    product("GFWRAP6", "Tortillas sin gluten", "6 uds", 2.95, "tortilla wrap sin gluten maiz", "harina de maíz, agua", ""),
    product("PB350", "Crema de cacahuete", "350 g", 2.80, "crema cacahuete peanut", "cacahuetes", "contiene cacahuetes"),
    product("SUNBUTTER300", "Crema de semillas de girasol", "300 g", 3.10, "crema semillas girasol sin frutos secos", "semillas de girasol", ""),
    product("OIL1L", "Aceite de oliva virgen extra", "1 L", 7.50, "aceite oliva", "aceite de oliva", ""),
    product("ONION1K", "Cebolla", "1 kg", 1.70, "cebolla verduras", "cebolla", "", "Verduras"),
    product("GARLIC250", "Ajo", "250 g", 1.55, "ajo verduras", "ajo", "", "Verduras"),
    product("CARROT1K", "Zanahorias", "1 kg", 1.45, "zanahoria verduras", "zanahorias", "", "Verduras"),
    product("PEPPER500", "Pimientos variados", "500 g", 2.30, "pimiento verduras", "pimientos", "", "Verduras"),
    product("ZUCCHINI1K", "Calabacín", "1 kg", 2.15, "calabacin verduras", "calabacín", "", "Verduras"),
    product("BEANS400", "Alubias rojas cocidas", "400 g", 1.05, "alubias judias rojas legumbres", "alubias rojas, agua, sal", ""),
    product("OATS500", "Copos de avena", "500 g", 1.65, "avena copos", "avena", "contiene gluten"),
    product("QUINOA500", "Quinoa", "500 g", 3.90, "quinoa grano", "quinoa", ""),
    product("LFCHEESE200", "Queso rallado sin lactosa", "200 g", 2.85, "queso sin lactosa rallado", "leche, fermentos; sin lactosa", "contiene leche", "Lácteos"),
    unavailable_product("SOLDOUT500", "Producto agotado de prueba", "500 g", 1.00, "agotado unavailable sold out test"),
]


def normalized(text):
    text = unicodedata.normalize("NFKD", text.lower())
    return "".join(char for char in text if not unicodedata.combining(char))


class State:
    def __init__(self, allow_generated=False):
        self.lock = threading.Lock()
        self.cart = {}
        self.dynamic = {}
        self.allow_generated = allow_generated

    def all_products(self):
        return CATALOG + list(self.dynamic.values())

    def search(self, query, limit):
        query_norm = normalized(query).strip()
        for item in self.all_products():
            # Live Alcampo search resolves SKUs but not internal UUIDs. Internal
            # IDs are resolved by the batch decoration endpoint instead.
            if query_norm == normalized(item["retailerProductId"]):
                return [item]
        tokens = [token for token in re.split(r"\W+", query_norm) if len(token) > 2]
        ranked = []
        for item in self.all_products():
            haystack = normalized(" ".join([item["name"], item.get("keywords", ""), item["retailerProductId"]]))
            score = sum(1 for token in tokens if token in haystack)
            if score:
                ranked.append((score, item))
        ranked.sort(key=lambda pair: (-pair[0], float(pair[1]["price"]["amount"])))
        if ranked:
            return [item for _, item in ranked[:limit]]
        if not self.allow_generated:
            return []
        key = hashlib.sha1(query_norm.encode()).hexdigest()[:10]
        if key not in self.dynamic:
            slug = " ".join(tokens) or "producto"
            self.dynamic[key] = product(
                f"GEN{key.upper()}",
                f"{slug.title()} (producto de prueba)",
                "500 g",
                2.00,
                query_norm,
                slug,
                "",
            )
        return [self.dynamic[key]]

    def by_ref(self, ref):
        ref_norm = normalized(str(ref))
        for item in self.all_products():
            if ref_norm in {normalized(item["productId"]), normalized(item["retailerProductId"])}:
                return item
        return None

    def cart_payload(self):
        with self.lock:
            quantities = dict(self.cart)
        items = []
        total = 0.0
        for product_id, quantity in sorted(quantities.items()):
            if quantity <= 0:
                continue
            item = self.by_ref(product_id)
            if not item:
                continue
            price = float(item["price"]["amount"])
            total += price * quantity
            items.append({
                "quantity": quantity,
                "lineTotal": {"amount": f"{price * quantity:.2f}", "currency": "EUR"},
                "product": item,
            })
        return {
            "totals": {"display": {"itemPriceAfterPromos": {"amount": f"{total:.2f}", "currency": "EUR"}}},
            "items": items,
        }


STATE = State()


class Handler(BaseHTTPRequestHandler):
    server_version = "CarritoMock/1.0"

    def log_message(self, fmt, *args):
        print(f"mock {self.command} {self.path} :: {fmt % args}", flush=True)

    def send_json(self, payload, status=200):
        raw = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def read_json(self):
        length = int(self.headers.get("Content-Length", "0"))
        return json.loads(self.rfile.read(length) or b"null")

    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path == "/":
            raw = b"<html><body>Carrito mock</body></html>"
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)
            return
        if parsed.path == "/__state":
            self.send_json(STATE.cart_payload())
            return
        if parsed.path == "/api/webproductpagews/v6/product-pages/search":
            query = parse_qs(parsed.query).get("q", [""])[0]
            limit = int(parse_qs(parsed.query).get("maxPageSize", ["10"])[0])
            self.send_json({"products": STATE.search(query, limit)})
            return
        if parsed.path in {"/api/cart/v2/carts/active/cart-view", "/api/cart/v1/carts/active"}:
            self.send_json(STATE.cart_payload())
            return
        if parsed.path.startswith("/products/"):
            self.send_json({"error": "mock detail page intentionally unavailable"}, 404)
            return
        self.send_json({"error": "not found", "path": parsed.path}, 404)

    def do_PUT(self):
        if urlparse(self.path).path == "/api/webproductpagews/v6/products":
            refs = self.read_json() or []
            self.send_json({"products": [item for ref in refs if (item := STATE.by_ref(ref))]})
            return
        self.send_json({"error": "not found"}, 404)

    def do_POST(self):
        path = urlparse(self.path).path
        if path == "/api/cart/v1/carts/active/apply-quantity":
            changes = self.read_json() or []
            with STATE.lock:
                for change in changes:
                    ref = change.get("productId", "")
                    if not STATE.by_ref(ref):
                        self.send_json({"error": f"unknown product {ref}"}, 400)
                        return
                    STATE.cart[ref] = max(0.0, STATE.cart.get(ref, 0.0) + float(change.get("quantity", 0)))
            self.send_json(STATE.cart_payload())
            return
        if path == "/__reset":
            with STATE.lock:
                STATE.cart.clear()
            self.send_json({"ok": True})
            return
        self.send_json({"error": "not found"}, 404)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8765)
    parser.add_argument("--allow-generated", action="store_true", help="synthesize fallback products; disabled by default so unresolved-query paths are testable")
    args = parser.parse_args()
    STATE.allow_generated = args.allow_generated
    server = ThreadingHTTPServer((args.host, args.port), Handler)
    print(f"mock Alcampo listening on http://{args.host}:{args.port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
