package http

// managing top and side menus

// structure for menu items
type menuItem struct {
	Name string // name of the menu item
	Link string // href link
	Icon string // name of the icon
}

// Menu item holder
type Menu struct {
	Name  string
	Items []*menuItem
}

// NewMenu , create an empty menu object
func NewMenu(name string) *Menu {
	m := &Menu{}
	m.Name = name
	return m
}

// AddItem , add a new item to the menu
func (m *Menu) AddItem(name string, link string, icon string) {
	newItem := &menuItem{name, link, icon}
	m.Items = append(m.Items, newItem)
}

// AddItem , add an item to the menu

// TODO
// Get , return sorted list
