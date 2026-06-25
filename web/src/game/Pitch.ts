// The playing surface: a striped (mown) ground plane with a field of short
// fluffy grass on top.
//
// The grass follows the Codrops "fluffiest grass" technique — a single
// InstancedMesh of tapered blades bent by a wind vertex shader, with a root→tip
// colour gradient in the fragment shader — tuned short so the field reads as a
// cut pitch rather than a meadow.

import {
  BufferGeometry,
  CanvasTexture,
  Color,
  DoubleSide,
  Float32BufferAttribute,
  Group,
  InstancedBufferAttribute,
  InstancedMesh,
  Matrix4,
  Mesh,
  MeshStandardMaterial,
  PlaneGeometry,
  Quaternion,
  ShaderMaterial,
  SRGBColorSpace,
  Vector2,
  Vector3,
} from 'three'
import { createPitchMarkings } from './PitchMarkings'

// Pitch dimensions. WIDTH is the long (goal-to-goal) axis on X; LENGTH is the
// short (touchline-to-touchline) axis on Z. This is the *marked* field: the
// boundary lines and goals sit at its edges, and it's shared with the camera
// and goals.
export const PITCH_WIDTH = 120
export const PITCH_LENGTH = 80

// Out-of-bounds margin. The mown ground and grass extend this far beyond the
// touchlines/goal lines on every side, so play can run off the marked field
// onto surrounding turf. The markings themselves stay at PITCH_WIDTH/LENGTH.
export const PITCH_MARGIN = 10

// Full ground surface = marked field + the out-of-bounds border all around.
export const GROUND_WIDTH = PITCH_WIDTH + PITCH_MARGIN * 2
export const GROUND_LENGTH = PITCH_LENGTH + PITCH_MARGIN * 2

// Grass. Short, mown-pitch blades scattered as a single InstancedMesh (one draw
// call). BLADE_HEIGHT is kept small so the field reads as a cut pitch rather
// than a meadow. Density is tuned against the field, then the total scales with
// the (larger) ground so the out-of-bounds border looks just as lush — bump the
// density for a fuller look, drop it on weaker GPUs.
const BLADE_WIDTH = 0.07
const BLADE_HEIGHT = 0.2
const GRASS_DENSITY = 50000 / (PITCH_WIDTH * PITCH_LENGTH) // blades per m²
const GRASS_COUNT = Math.round(GRASS_DENSITY * GROUND_WIDTH * GROUND_LENGTH)

export interface Pitch {
  group: Group
  /** Advance the wind animation. `elapsed` is seconds since start. */
  update: (elapsed: number) => void
  dispose: () => void
}

export function createPitch(maxAnisotropy: number): Pitch {
  const group = new Group()

  // Striped (mown) ground plane. Sized to the full ground (field + margin) so
  // there's turf beyond the markings to run out of bounds onto. The stripes
  // live in a generated texture used as the map, so standard lighting applies.
  const groundGeometry = new PlaneGeometry(GROUND_WIDTH, GROUND_LENGTH)
  const pitchTexture = createPitchTexture(maxAnisotropy)
  const groundMaterial = new MeshStandardMaterial({ color: 0xffffff, map: pitchTexture })
  const ground = new Mesh(groundGeometry, groundMaterial)
  ground.rotation.x = -Math.PI / 2
  group.add(ground)

  // Painted FIFA markings. Built before the grass so we can keep blades off the
  // lines (isMarked) — otherwise the short grass buries them from this angle.
  const markings = createPitchMarkings(PITCH_WIDTH, PITCH_LENGTH)
  group.add(markings.mesh)

  // Grass: GRASS_COUNT instanced blades scattered over the pitch, each with a
  // random position, yaw, and size, and a per-blade random (aRand) that offsets
  // its wind phase and tints. A custom ShaderMaterial does the wind + gradient.
  const bladeGeometry = createBladeGeometry()
  const grassMaterial = new ShaderMaterial({
    vertexShader: GRASS_VERT,
    fragmentShader: GRASS_FRAG,
    side: DoubleSide,
    uniforms: {
      uTime: { value: 0 },
      uWindDir: { value: new Vector2(0.8, 0.6).normalize() },
      uWindStrength: { value: 0.09 },
      uWindFreq: { value: 1.6 },
      uRootColor: { value: new Color(0x2c6e2c) },
      uTipColor: { value: new Color(0x7ad15f) },
    },
  })
  const grass = new InstancedMesh(bladeGeometry, grassMaterial, GRASS_COUNT)
  grass.frustumCulled = false // one field-sized object; never cull it whole

  const aRand = new Float32Array(GRASS_COUNT)
  const m = new Matrix4()
  const q = new Quaternion()
  const pos = new Vector3()
  const scl = new Vector3()
  const up = new Vector3(0, 1, 0)
  const halfW = GROUND_WIDTH / 2
  const halfL = GROUND_LENGTH / 2
  for (let i = 0; i < GRASS_COUNT; i++) {
    const x = (Math.random() * 2 - 1) * halfW
    const z = (Math.random() * 2 - 1) * halfL
    pos.set(x, 0, z)
    q.setFromAxisAngle(up, Math.random() * Math.PI * 2)
    // Collapse blades that land on a painted line so the markings stay visible.
    if (markings.isMarked(x, z)) scl.set(0, 0, 0)
    else scl.set(0.8 + Math.random() * 0.5, 0.7 + Math.random() * 0.5, 1) // jitter width & height
    m.compose(pos, q, scl)
    grass.setMatrixAt(i, m)
    aRand[i] = Math.random()
  }
  grass.instanceMatrix.needsUpdate = true
  bladeGeometry.setAttribute('aRand', new InstancedBufferAttribute(aRand, 1))
  group.add(grass)

  return {
    group,
    update(elapsed) {
      grassMaterial.uniforms.uTime.value = elapsed
    },
    dispose() {
      groundGeometry.dispose()
      groundMaterial.dispose()
      pitchTexture.dispose()
      markings.dispose()
      bladeGeometry.dispose()
      grassMaterial.dispose()
      grass.dispose()
    },
  }
}

// A blade is a thin, tapered vertical strip with a few height segments so the
// wind shader can bend it smoothly. Base sits at y=0; uv.y runs 0 (root) → 1
// (tip) and drives both the bend amount and the colour gradient.
function createBladeGeometry(): BufferGeometry {
  const SEGMENTS = 4
  const positions: number[] = []
  const uvs: number[] = []
  const indices: number[] = []
  const halfW = BLADE_WIDTH / 2

  for (let i = 0; i <= SEGMENTS; i++) {
    const t = i / SEGMENTS
    const y = t * BLADE_HEIGHT
    const w = halfW * (1 - t) // taper to a point at the tip
    positions.push(-w, y, 0, w, y, 0)
    uvs.push(0, t, 1, t)
  }
  for (let i = 0; i < SEGMENTS; i++) {
    const a = i * 2
    indices.push(a, a + 2, a + 1, a + 1, a + 2, a + 3)
  }

  const geo = new BufferGeometry()
  geo.setAttribute('position', new Float32BufferAttribute(positions, 3))
  geo.setAttribute('uv', new Float32BufferAttribute(uvs, 2))
  geo.setIndex(indices)
  return geo
}

// Procedural pitch texture: classic alternating mowing stripes (vertical bands
// across the long axis, so they read as broadcast stripes) plus a fine speckle
// so the ground isn't a flat fill.
function createPitchTexture(maxAnisotropy: number): CanvasTexture {
  const canvas = document.createElement('canvas')
  canvas.width = 1024
  canvas.height = 1024
  const ctx = canvas.getContext('2d')!

  const STRIPES = 16
  const bandW = canvas.width / STRIPES
  for (let i = 0; i < STRIPES; i++) {
    ctx.fillStyle = i % 2 === 0 ? '#4a9e4a' : '#357f35'
    ctx.fillRect(i * bandW, 0, Math.ceil(bandW), canvas.height)
  }
  // Subtle light/dark speckle so the turf has grain from a distance.
  for (let n = 0; n < 24000; n++) {
    const a = Math.random() * 0.06
    ctx.fillStyle = Math.random() < 0.5 ? `rgba(0,0,0,${a})` : `rgba(255,255,255,${a})`
    ctx.fillRect(Math.random() * canvas.width, Math.random() * canvas.height, 2, 2)
  }

  const tex = new CanvasTexture(canvas)
  tex.colorSpace = SRGBColorSpace
  tex.anisotropy = maxAnisotropy
  return tex
}

// Bends each blade in world space along a fixed wind direction, weighted by
// uv.y² so the root stays planted and the tip travels most. Two summed sines
// (offset per blade via aRand) keep the field from waving in unison.
const GRASS_VERT = /* glsl */ `
  uniform float uTime;
  uniform vec2 uWindDir;
  uniform float uWindStrength;
  uniform float uWindFreq;
  attribute float aRand;
  varying vec2 vUv;
  varying float vRand;

  void main() {
    vUv = uv;
    vRand = aRand;
    vec4 worldPosition = modelMatrix * instanceMatrix * vec4(position, 1.0);
    float bend = uv.y * uv.y;
    float phase = aRand * 6.2831853;
    float wind = sin(uTime * uWindFreq + worldPosition.x * 0.15 + worldPosition.z * 0.18 + phase);
    wind += 0.5 * sin(uTime * uWindFreq * 1.9 + phase);
    worldPosition.xz += uWindDir * wind * uWindStrength * bend;
    gl_Position = projectionMatrix * viewMatrix * worldPosition;
  }
`

// Root→tip colour lerp, a per-blade brightness jitter, and a fake ambient
// occlusion that darkens the base so the turf has depth.
const GRASS_FRAG = /* glsl */ `
  uniform vec3 uRootColor;
  uniform vec3 uTipColor;
  varying vec2 vUv;
  varying float vRand;

  void main() {
    vec3 col = mix(uRootColor, uTipColor, vUv.y);
    col *= 0.82 + 0.36 * vRand;
    col *= mix(0.55, 1.0, vUv.y);
    gl_FragColor = vec4(col, 1.0);
    #include <colorspace_fragment>
  }
`
