import { describe, expect, it } from 'vitest'
import { pelicanGalleryLayout } from '../usePelicanGalleryLayout'
describe('Pelican gallery available container', () => {
  it('fits two rows in normal and wide desktop content areas', () => {
    expect(pelicanGalleryLayout(1136, 600)).toMatchObject({
      columns: 4,
      rows: 2,
      pageSize: 8
    })
    expect(pelicanGalleryLayout(1616, 600)).toMatchObject({
      columns: 6,
      rows: 2,
      pageSize: 12
    })
  })
  it('fits list rows to available height', () => {
    expect(pelicanGalleryLayout(1136, 610, true)).toMatchObject({
      compact: true,
      columns: 1,
      rows: 5,
      pageSize: 5
    })
    expect(pelicanGalleryLayout(1136, 350, true).pageSize).toBe(3)
  })
  it('uses horizontal mobile rows and reduces narrow or short layouts', () => {
    expect(pelicanGalleryLayout(366, 500)).toMatchObject({
      compact: true,
      rows: 4,
      pageSize: 4
    })
    expect(pelicanGalleryLayout(366, 350)).toMatchObject({ compact: true, rows: 3, pageSize: 3 })
    expect(pelicanGalleryLayout(296, 330)).toMatchObject({
      compact: true,
      rows: 3,
      pageSize: 3
    })
    expect(pelicanGalleryLayout(1136, 350).pageSize).toBe(4)
  })
})
