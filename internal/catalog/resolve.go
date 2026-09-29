package catalog

import "fmt"

// Resolved is a profile flattened: extends followed, apps de-duplicated and
// ordered so that dependencies (requires) come first.
type Resolved struct {
	Apps    []ProfileApp
	Recipes []string
}

// Resolve flattens a catalog profile.
func (c *Catalog) Resolve(id string) (*Resolved, error) {
	p := c.Profiles[id]
	if p == nil {
		return nil, fmt.Errorf("unknown profile %q", id)
	}
	return c.ResolveProfile(p)
}

// ResolveProfile flattens any profile, including an imported one that is not
// part of the catalog, against the catalog's apps and profiles. It rejects
// extends and requires cycles and any id the catalog does not know.
func (c *Catalog) ResolveProfile(p *Profile) (*Resolved, error) {
	res := &Resolved{Apps: []ProfileApp{}, Recipes: []string{}}
	seenApp := map[string]bool{}
	seenRecipe := map[string]bool{}
	visiting := map[string]bool{}

	var addApp func(pa ProfileApp, chain []string) error
	addApp = func(pa ProfileApp, chain []string) error {
		a := c.Apps[pa.ID]
		if a == nil {
			return fmt.Errorf("unknown app %q", pa.ID)
		}
		for _, id := range chain {
			if id == pa.ID {
				return fmt.Errorf("requires cycle through %q", pa.ID)
			}
		}
		if seenApp[pa.ID] {
			return nil
		}
		for _, req := range a.Requires {
			if err := addApp(ProfileApp{ID: req}, append(chain, pa.ID)); err != nil {
				return err
			}
		}
		seenApp[pa.ID] = true
		res.Apps = append(res.Apps, pa)
		return nil
	}
	addRecipe := func(id string) error {
		if c.Recipes[id] == nil {
			return fmt.Errorf("unknown recipe %q", id)
		}
		if !seenRecipe[id] {
			seenRecipe[id] = true
			res.Recipes = append(res.Recipes, id)
		}
		return nil
	}

	var walk func(p *Profile) error
	walk = func(p *Profile) error {
		if visiting[p.ID] {
			return fmt.Errorf("extends cycle through %q", p.ID)
		}
		visiting[p.ID] = true
		defer delete(visiting, p.ID)
		for _, e := range p.Extends {
			parent := c.Profiles[e]
			if parent == nil {
				return fmt.Errorf("extends unknown profile %q", e)
			}
			if err := walk(parent); err != nil {
				return err
			}
		}
		for _, pa := range p.Apps {
			if err := addApp(pa, nil); err != nil {
				return err
			}
		}
		for _, r := range p.Recipes {
			if err := addRecipe(r); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(p); err != nil {
		return nil, err
	}
	// Recipes attached to an app run whenever the app is part of the plan.
	for _, pa := range res.Apps {
		for _, r := range c.Apps[pa.ID].Recipes {
			if err := addRecipe(r); err != nil {
				return nil, err
			}
		}
	}
	return res, nil
}
