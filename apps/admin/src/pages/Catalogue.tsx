import { FormEvent, useEffect, useMemo, useState } from "react";
import { apiGet, apiPatch, apiPost } from "../api/client";
import { useAuth } from "../auth";
import { Badge, Card, Drawer, EmptyState, FilterBar, Flash, FormGrid, LoadingState, Page, SearchField } from "../components/ui";
import { humanStatus, money, nairaToMinor, slugify, type Row } from "../lib/format";
import { hasPermission } from "../lib/permissions";

export function CataloguePage() {
  const { user } = useAuth();
  const canManageCatalogue = hasPermission(user.role, "catalogue.manage");
  const [categories, setCategories] = useState<Row[]>([]);
  const [products, setProducts] = useState<Row[]>([]);
  const [query, setQuery] = useState("");
  const [categoryFilter, setCategoryFilter] = useState("all");
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");
  const [open, setOpen] = useState(false);
  const [categoriesOpen, setCategoriesOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<Row | null>(null);
  const [categoryForm, setCategoryForm] = useState({ name: "" });
  const [form, setForm] = useState({ name: "", description: "", category_id: "", price: "", image_url: "", available: true, variant: "Regular" });

  async function load() {
    try {
      const [categoryResponse, productResponse] = await Promise.all([apiGet<Row[]>("/catalogue/categories"), apiGet<Row[]>("/catalogue/products")]);
      setCategories(categoryResponse.data);
      setProducts(productResponse.data);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load catalogue");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function startCreate() {
    setEditing(null);
    setForm({ name: "", description: "", category_id: categories[0] ? String(categories[0].id) : "", price: "", image_url: "", available: true, variant: "Regular" });
    setOpen(true);
  }

  function startEdit(product: Row) {
    const variants = Array.isArray(product.variants) ? (product.variants as Row[]) : [];
    const images = Array.isArray(product.images) ? (product.images as Row[]) : [];
    setEditing(product);
    setForm({
      name: String(product.name ?? ""),
      description: String(product.description ?? ""),
      category_id: String(product.category_id ?? ""),
      price: variants[0] ? String(Number(variants[0].price_minor ?? 0) / 100) : "",
      image_url: String(images[0]?.url ?? ""),
      available: product.status !== "inactive",
      variant: String(variants[0]?.name ?? "Regular"),
    });
    setOpen(true);
  }

  async function createCategory(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>("/catalogue/categories", { name: categoryForm.name, slug: slugify(categoryForm.name), status: "active" });
    setCategoryForm({ name: "" });
    setFlash("Category added.");
    await load();
  }

  async function saveProduct(event: FormEvent) {
    event.preventDefault();
    const payload = {
      name: form.name,
      slug: slugify(form.name),
      description: form.description,
      category_id: form.category_id || undefined,
      status: form.available ? "active" : "inactive",
    };
    if (editing) {
      await apiPatch<Row>(`/catalogue/products/${String(editing.id)}`, payload);
      if (form.image_url && form.image_url !== String(((editing.images as Row[]) ?? [])[0]?.url ?? "")) {
        await apiPost<Row>(`/catalogue/products/${String(editing.id)}/images`, { url: form.image_url, alt_text: form.name });
      }
      setFlash("Product updated.");
    } else {
      await apiPost<Row>("/catalogue/products", {
        ...payload,
        variants: [{ sku: slugify(form.name).toUpperCase() || "SKU", name: form.variant || "Regular", price_minor: nairaToMinor(form.price), currency: "NGN", status: "active" }],
        images: form.image_url ? [{ url: form.image_url, alt_text: form.name }] : [],
      });
      setFlash("Product added.");
    }
    setOpen(false);
    await load();
  }

  const filtered = useMemo(() => {
    return products.filter((product) => {
      const matchesQuery = !query || String(product.name).toLowerCase().includes(query.toLowerCase());
      const matchesCategory = categoryFilter === "all" || String(product.category_id) === categoryFilter;
      return matchesQuery && matchesCategory;
    });
  }, [products, query, categoryFilter]);

  return (
    <Page
      title="Catalogue"
      description="The products and variants customers can order through your business."
      actions={<><button type="button" onClick={() => setCategoriesOpen(true)}>Categories</button>{canManageCatalogue ? <button type="button" className="primary" onClick={startCreate}>Add product</button> : null}</>}
    >
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <FilterBar>
        <SearchField value={query} onChange={setQuery} placeholder="Search products" />
        <select aria-label="Filter by category" value={categoryFilter} onChange={(event) => setCategoryFilter(event.target.value)}>
          <option value="all">All categories</option>
          {categories.map((category) => <option key={String(category.id)} value={String(category.id)}>{String(category.name)}</option>)}
        </select>
      </FilterBar>
      <Drawer open={categoriesOpen} title="Categories" onClose={() => setCategoriesOpen(false)}>
        {canManageCatalogue ? <FormGrid onSubmit={createCategory}>
          <label className="full">Category name<input value={categoryForm.name} onChange={(event) => setCategoryForm({ name: event.target.value })} required /></label>
          <div className="full"><button className="primary" type="submit">Add category</button></div>
        </FormGrid> : null}
        {categories.length === 0 ? <EmptyState title="No categories" body="Group your products so customers can find them." /> : <ul className="category-list">{categories.map((category) => <li key={String(category.id)}><span>{String(category.name)}</span><span className="muted">{products.filter(product => product.category_id === category.id).length} products</span></li>)}</ul>}
      </Drawer>
      <p className="result-count">{filtered.length} product{filtered.length === 1 ? "" : "s"}</p>
      {loading ? <LoadingState label="Loading catalogue" /> : filtered.length === 0 ? (
        <EmptyState title={products.length ? "No matching products" : "No products yet"} body={products.length ? "Try another search or category." : "Add your first product to start taking orders."} action={canManageCatalogue ? <button type="button" className="primary" onClick={startCreate}>Add product</button> : undefined} />
      ) : (
        <Card className="orders-register"><div className="table-wrap" style={{ border: 0, margin: 0 }}><table>
          <thead><tr><th>Product</th><th>Category</th><th>Price</th><th>Options</th><th>Status</th>{canManageCatalogue ? <th><span className="sr-only">Actions</span></th> : null}</tr></thead>
          <tbody>{filtered.map((product) => {
            const variants = Array.isArray(product.variants) ? (product.variants as Row[]) : [];
            const images = Array.isArray(product.images) ? (product.images as Row[]) : [];
            const price = variants[0]?.price_minor;
            const categoryName = categories.find((entry) => entry.id === product.category_id)?.name;
            return <tr key={String(product.id)}>
              <td><div className="product-cell">{images[0]?.url ? <img src={String(images[0].url)} alt="" loading="lazy" /> : <span className="product-placeholder" aria-hidden="true">{String(product.name).slice(0,1)}</span>}<span>{canManageCatalogue ? <button className="table-record-link" type="button" onClick={() => startEdit(product)}>{String(product.name)}</button> : String(product.name)}</span></div></td>
              <td>{String(categoryName || "Uncategorised")}</td><td>{price !== undefined ? money(price, String(variants[0]?.currency ?? "NGN")) : "No price"}</td><td>{variants.length}</td><td><Badge tone={product.status === "active" ? "success" : "neutral"}>{humanStatus(product.status)}</Badge></td>
              {canManageCatalogue ? <td><button type="button" onClick={() => startEdit(product)} aria-label={`Edit ${product.name}`}>Edit</button></td> : null}
            </tr>;
          })}</tbody>
        </table></div></Card>
      )}
      <Drawer open={open} title={editing ? "Edit product" : "Add product"} onClose={() => setOpen(false)}>
        <FormGrid onSubmit={saveProduct}>
          <label className="full">Name<input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required /></label>
          <label className="full">Description<textarea value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} /></label>
          <label>Category
            <select value={form.category_id} onChange={(event) => setForm({ ...form, category_id: event.target.value })}>
              <option value="">Uncategorised</option>
              {categories.map((category) => <option key={String(category.id)} value={String(category.id)}>{String(category.name)}</option>)}
            </select>
          </label>
          {!editing ? <label>Price (NGN)<input type="number" min="0" value={form.price} onChange={(event) => setForm({ ...form, price: event.target.value })} required /></label> : null}
          <label className="full">Image URL<input value={form.image_url} onChange={(event) => setForm({ ...form, image_url: event.target.value })} placeholder="https://" /></label>
          <label className="full">Available?
            <select value={form.available ? "yes" : "no"} onChange={(event) => setForm({ ...form, available: event.target.value === "yes" })}>
              <option value="yes">Yes, customers can order this</option>
              <option value="no">Hidden from the assistant</option>
            </select>
          </label>
          {editing ? <p className="help-text full">Change prices in Options below. Customers see published catalogue immediately; the assistant uses these products when they order.</p> : null}
          <div className="full"><button type="submit">{editing ? "Save product" : "Create product"}</button></div>
        </FormGrid>
        {editing && canManageCatalogue ? (
          <VariantEditor product={editing} onSaved={async () => { setFlash("Option saved."); await load(); }} />
        ) : null}
      </Drawer>
    </Page>
  );
}

function VariantEditor({ product, onSaved }: { product: Row; onSaved: () => Promise<void> }) {
  const variants = Array.isArray(product.variants) ? (product.variants as Row[]) : [];
  const [name, setName] = useState("");
  const [price, setPrice] = useState("");
  const [editPrices, setEditPrices] = useState<Record<string, string>>({});

  async function addVariant(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>(`/catalogue/products/${String(product.id)}/variants`, {
      sku: slugify(`${product.name}-${name}`).toUpperCase(),
      name,
      price_minor: nairaToMinor(price),
      currency: "NGN",
      status: "active",
    });
    setName("");
    setPrice("");
    await onSaved();
  }

  async function savePrice(variant: Row) {
    const next = editPrices[String(variant.id)] ?? String(Number(variant.price_minor ?? 0) / 100);
    await apiPatch<Row>(`/catalogue/variants/${String(variant.id)}`, { price_minor: nairaToMinor(next) });
    await onSaved();
  }

  return (
    <div>
      <h3>Options and prices</h3>
      {variants.map((variant) => (
        <div key={String(variant.id)} className="filter-bar" style={{ marginBottom: 8 }}>
          <span>{String(variant.name)}</span>
          <input
            aria-label={`Price for ${variant.name}`}
            type="number"
            min="0"
            value={editPrices[String(variant.id)] ?? String(Number(variant.price_minor ?? 0) / 100)}
            onChange={(event) => setEditPrices({ ...editPrices, [String(variant.id)]: event.target.value })}
          />
          <button type="button" onClick={() => void savePrice(variant)}>Save price</button>
        </div>
      ))}
      <form className="form-grid" onSubmit={addVariant}>
        <strong>Add option</strong>
        <label>Name<input value={name} onChange={(event) => setName(event.target.value)} placeholder="Large" required /></label>
        <label>Price (NGN)<input type="number" min="0" value={price} onChange={(event) => setPrice(event.target.value)} required /></label>
        <div className="full"><button type="submit">Add option</button></div>
      </form>
    </div>
  );
}
